package cluster

import (
	"github.com/lucas-ht/keldock/internal/compiler"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// selectedJobFailure uses terminal Job evidence even while Flux retries its
// health check. A failed pod count alone does not establish permanent Job failure.
func selectedJobFailure(job *unstructured.Unstructured, name, image string, selection compiler.Selection) bool {
	if job.GetName() != compiler.JobName(name, selection.DeploymentID) || job.GetNamespace() != selection.Namespace || job.GetLabels()["keldock.io/environment"] != selection.EnvironmentID || job.GetAnnotations()["keldock.io/deployment"] != selection.DeploymentID {
		return false
	}
	containers, _, _ := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	if len(containers) != 1 {
		return false
	}
	container, ok := containers[0].(map[string]any)
	if !ok || container["image"] != image || container["name"] != name {
		return false
	}
	conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if ok && condition["type"] == "Failed" && condition["status"] == "True" {
			return true
		}
	}
	return false
}
