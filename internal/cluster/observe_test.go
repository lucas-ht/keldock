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

func TestRetryKeepsOnlyCurrentLatestCompletedFailure(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name, revision, status, state string
		observed                      int64
		failed                        bool
	}{
		{"failed retry", digest, "HealthCheckFailed", "Unknown", 3, true},
		{"older revision", "sha256:" + strings.Repeat("b", 64), "HealthCheckFailed", "Unknown", 3, false},
		{"older specification", digest, "HealthCheckFailed", "Unknown", 2, false},
		{"completed success", digest, "ReconciliationSucceeded", "Unknown", 3, false},
		{"current ready", digest, "HealthCheckFailed", "True", 3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			object := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"generation": int64(3)}, "status": map[string]any{"lastAttemptedRevision": digest, "observedGeneration": test.observed, "conditions": []any{map[string]any{"type": "Ready", "status": test.state, "reason": "Progressing", "observedGeneration": int64(3)}}, "history": []any{map[string]any{"metadata": map[string]any{"revision": test.revision}, "lastReconciledStatus": test.status}}}}}
			if selectedFailure(object, digest) != test.failed {
				t.Fatal("retry history lost revision/specification isolation")
			}
		})
	}
}
