package cluster

import (
	"context"

	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (registrar *Registrar) workloadReady(ctx context.Context, name string, component v1.Component, image string, selection compiler.Selection, health *v1.ComponentHealth) (bool, error) {
	kind, objectName := "Deployment", name
	podPath := []string{"spec", "template", "spec"}
	if component.Kind == "job" {
		kind = "Job"
		objectName = compiler.JobName(name, selection.DeploymentID)
	}
	if component.Kind == "cronjob" {
		kind = "CronJob"
		podPath = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	object, err := registrar.resource(kind, selection.Namespace).Get(ctx, objectName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if object.GetLabels()["keldock.io/environment"] != selection.EnvironmentID || object.GetAnnotations()["keldock.io/deployment"] != selection.DeploymentID {
		return false, nil
	}
	health.WorkloadUID = string(object.GetUID())
	health.WorkloadVersion = object.GetResourceVersion()
	containers, _, _ := unstructured.NestedSlice(object.Object, append(podPath, "containers")...)
	if len(containers) != 1 {
		return false, nil
	}
	container, ok := containers[0].(map[string]any)
	if !ok || container["image"] != image {
		return false, nil
	}
	health.Image = image
	if kind == "Deployment" {
		observed, _, _ := unstructured.NestedInt64(object.Object, "status", "observedGeneration")
		replicas, _, _ := unstructured.NestedInt64(object.Object, "spec", "replicas")
		if observed != object.GetGeneration() || replicas < 1 {
			return false, nil
		}
		for _, field := range []string{"replicas", "updatedReplicas", "availableReplicas", "readyReplicas"} {
			count, _, _ := unstructured.NestedInt64(object.Object, "status", field)
			if count != replicas {
				return false, nil
			}
		}
	}
	if kind == "Job" {
		complete := false
		conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		for _, value := range conditions {
			condition, ok := value.(map[string]any)
			if ok && condition["type"] == "Complete" && condition["status"] == "True" {
				complete = true
			}
		}
		if !complete {
			return false, nil
		}
	}
	if component.Kind == "http" {
		for _, kind := range []string{"Service", "Ingress"} {
			if kind == "Ingress" && !component.Route {
				continue
			}
			resource, err := registrar.resource(kind, selection.Namespace).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if resource.GetAnnotations()["keldock.io/deployment"] != selection.DeploymentID {
				return false, nil
			}
			if kind == "Ingress" {
				addresses, _, _ := unstructured.NestedSlice(resource.Object, "status", "loadBalancer", "ingress")
				if len(addresses) == 0 {
					return false, nil
				}
			}
		}
	}
	return true, nil
}
