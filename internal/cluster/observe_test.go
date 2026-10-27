package cluster

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFailureMustBelongToSelectedArtifactAndGeneration(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name       string
		revision   string
		generation int64
		failed     bool
	}{
		{"current failed attempt", digest, 3, true},
		{"previous artifact failure", "sha256:" + strings.Repeat("b", 64), 3, false},
		{"previous specification failure", digest, 2, false},
		{"no attempted artifact", "", 3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			object := &unstructured.Unstructured{Object: map[string]any{
				"metadata": map[string]any{"generation": int64(3)},
				"status":   map[string]any{"lastAttemptedRevision": test.revision, "conditions": []any{map[string]any{"type": "Ready", "status": "False", "reason": "HealthCheckFailed", "observedGeneration": test.generation}}},
			}}
			if selectedFailure(object, digest) != test.failed {
				t.Fatal("failure from a different selection affected the current outcome")
			}
		})
	}
}
