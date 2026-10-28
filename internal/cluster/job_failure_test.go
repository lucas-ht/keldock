package cluster

import (
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/compiler"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTerminalJobFailureRequiresSelectedWorkload(t *testing.T) {
	selection := compiler.Selection{Namespace: "ke-test", EnvironmentID: "env-test", DeploymentID: "dep-current"}
	image := "registry.example/migration@sha256:" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name   string
		mutate func(*unstructured.Unstructured)
		failed bool
	}{
		{"terminal selected job", func(*unstructured.Unstructured) {}, true},
		{"old deployment", func(job *unstructured.Unstructured) {
			job.SetAnnotations(map[string]string{"keldock.io/deployment": "dep-old"})
		}, false},
		{"another environment", func(job *unstructured.Unstructured) {
			job.SetLabels(map[string]string{"keldock.io/environment": "env-other"})
		}, false},
		{"another job name", func(job *unstructured.Unstructured) { job.SetName("unrelated") }, false},
		{"another namespace", func(job *unstructured.Unstructured) { job.SetNamespace("ke-other") }, false},
		{"another image", func(job *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(job.Object, []any{map[string]any{"name": "migrate", "image": "unapproved"}}, "spec", "template", "spec", "containers")
		}, false},
		{"failed pods without terminal condition", func(job *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(job.Object, []any{}, "status", "conditions")
		}, false},
		{"successful job", func(job *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(job.Object, []any{map[string]any{"type": "Complete", "status": "True"}}, "status", "conditions")
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			job := &unstructured.Unstructured{Object: map[string]any{
				"metadata": map[string]any{"name": compiler.JobName("migrate", selection.DeploymentID), "namespace": selection.Namespace, "labels": map[string]any{"keldock.io/environment": selection.EnvironmentID}, "annotations": map[string]any{"keldock.io/deployment": selection.DeploymentID}},
				"spec":     map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "migrate", "image": image}}}}},
				"status":   map[string]any{"failed": int64(1), "conditions": []any{map[string]any{"type": "Failed", "status": "True"}}},
			}}
			test.mutate(job)
			if selectedJobFailure(job, "migrate", image, selection) != test.failed {
				t.Fatal("terminal job evidence crossed the selected workload boundary")
			}
		})
	}
}
