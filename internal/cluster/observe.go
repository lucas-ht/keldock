package cluster

import (
	"context"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func condition(object *unstructured.Unstructured) (string, string, string) {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, value := range conditions {
		entry, ok := value.(map[string]any)
		if !ok || entry["type"] != "Ready" {
			continue
		}
		state, _ := entry["status"].(string)
		reason, _ := entry["reason"].(string)
		message, _ := entry["message"].(string)
		if len(message) > 1024 {
			message = message[:1024]
		}
		return state, reason, message
	}
	return "Unknown", "NotObserved", ""
}

func selectedFailure(object *unstructured.Unstructured, digest string) bool {
	attempted, _, _ := unstructured.NestedString(object.Object, "status", "lastAttemptedRevision")
	if !strings.HasSuffix(attempted, digest) {
		return false
	}
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, value := range conditions {
		entry, ok := value.(map[string]any)
		if !ok || entry["type"] != "Ready" || entry["status"] != "False" || entry["observedGeneration"] != object.GetGeneration() {
			continue
		}
		if entry["reason"] == "HealthCheckFailed" || entry["reason"] == "BuildFailed" || entry["reason"] == "ReconciliationFailed" {
			return true
		}
	}
	// A failed attempt can be replaced by Progressing as soon as Flux retries.
	// Preserve only the latest completed attempt for this exact source revision
	// and already-observed specification; an older failure cannot taint a new one.
	state, _, _ := condition(object)
	observed, _, _ := unstructured.NestedInt64(object.Object, "status", "observedGeneration")
	if state == "True" || observed != object.GetGeneration() {
		return false
	}
	history, _, _ := unstructured.NestedSlice(object.Object, "status", "history")
	if len(history) == 0 {
		return false
	}
	latest, ok := history[0].(map[string]any)
	if !ok {
		return false
	}
	revision, _, _ := unstructured.NestedString(latest, "metadata", "revision")
	if !strings.HasSuffix(revision, digest) {
		return false
	}
	return latest["lastReconciledStatus"] == "HealthCheckFailed" || latest["lastReconciledStatus"] == "BuildFailed" || latest["lastReconciledStatus"] == "ReconciliationFailed"
}

// Observe requires the selected artifact and current controller generations, never old readiness.
func (registrar *Registrar) Observe(ctx context.Context, selection compiler.Selection, definition v1.Application, images map[string]string, bundle string) (v1.Observation, error) {
	report := v1.Observation{EnvironmentID: selection.EnvironmentID, DeploymentID: selection.DeploymentID, Generation: selection.Generation, Bundle: bundle, State: "ready", Reason: "Reconciled", ObservedAt: time.Now().UTC()}
	_, digest, err := artifacts.SplitReference(bundle)
	if err != nil {
		return report, err
	}
	source, err := registrar.resource("OCIRepository", selection.Namespace).Get(ctx, "keldock-source", metav1.GetOptions{})
	if err != nil {
		return report, err
	}
	sourceState, _, _ := condition(source)
	sourceGeneration, _, _ := unstructured.NestedInt64(source.Object, "status", "observedGeneration")
	revision, _, _ := unstructured.NestedString(source.Object, "status", "artifact", "revision")
	sourceReady := sourceState == "True" && sourceGeneration == source.GetGeneration() && strings.HasSuffix(revision, digest)
	names, err := definition.Order()
	if err != nil {
		return report, err
	}
	for _, name := range names {
		health := v1.ComponentHealth{Name: name, Reason: "NotRegistered"}
		object, err := registrar.resource("Kustomization", selection.Namespace).Get(ctx, "keldock-"+name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return report, err
		}
		if err == nil {
			state, reason, message := condition(object)
			health.Reason, health.Message = reason, message
			health.ObjectUID = string(object.GetUID())
			health.ResourceVersion = object.GetResourceVersion()
			health.ObservedGeneration, _, _ = unstructured.NestedInt64(object.Object, "status", "observedGeneration")
			health.AppliedRevision, _, _ = unstructured.NestedString(object.Object, "status", "lastAppliedRevision")
			health.Ready = sourceReady && state == "True" && health.ObservedGeneration == object.GetGeneration() && strings.HasSuffix(health.AppliedRevision, digest) && object.GetLabels()["keldock.io/deployment"] == selection.DeploymentID
			if !health.Ready && state == "True" {
				health.Reason = "AwaitingSelectedRevision"
			}
			if health.Ready {
				workloadReady, err := registrar.workloadReady(ctx, name, definition.Components[name], images[name], selection, &health)
				if err != nil {
					return report, err
				}
				health.Ready = workloadReady
				if !workloadReady {
					health.Reason = "AwaitingSelectedWorkload"
				}
			}
			if health.Ready && definition.Components[name].Kind == "cronjob" {
				health.Reason = "ScheduleConfigured"
				health.Message = "Schedule configured; this does not prove an execution completed."
			}
			jobFailed := false
			if !health.Ready && sourceReady && definition.Components[name].Kind == "job" && object.GetLabels()["keldock.io/deployment"] == selection.DeploymentID {
				job, err := registrar.resource("Job", selection.Namespace).Get(ctx, compiler.JobName(name, selection.DeploymentID), metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return report, err
				}
				if err == nil && selectedJobFailure(job, name, images[name], selection) {
					jobFailed = true
					report.State, report.Reason = "failed", "SelectedJobFailed"
					health.Reason, health.Message = "SelectedJobFailed", "The selected deployment's job reached a terminal failure. Submit a corrected release to retry."
					health.WorkloadUID, health.WorkloadVersion, health.Image = string(job.GetUID()), job.GetResourceVersion(), images[name]
				}
			}
			if !health.Ready && !jobFailed && selectedFailure(object, digest) {
				report.State = "failed"
				report.Reason = reason
				if reason == "Progressing" || reason == "ProgressingWithRetry" {
					report.Reason = "SelectedRevisionFailedRetrying"
					health.Reason = report.Reason
					health.Message = "The last completed reconciliation of this selected revision failed; Flux is retrying."
				}
			}
		}
		if !health.Ready && report.State != "failed" {
			report.State = "reconciling"
			report.Reason = "AwaitingComponents"
		}
		report.Components = append(report.Components, health)
	}
	objects, err := registrar.resource("Kustomization", selection.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "keldock.io/environment=" + selection.EnvironmentID})
	if err != nil {
		return report, err
	}
	if len(objects.Items) != len(names) && report.State == "ready" {
		report.State = "reconciling"
		report.Reason = "WaitingForPrune"
	}
	return report, nil
}
