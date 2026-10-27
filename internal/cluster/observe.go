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
		return entry["reason"] == "HealthCheckFailed" || entry["reason"] == "BuildFailed" || entry["reason"] == "ReconciliationFailed"
	}
	return false
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
			if !health.Ready && selectedFailure(object, digest) {
				report.State = "failed"
				report.Reason = reason
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
