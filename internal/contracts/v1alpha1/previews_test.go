package v1alpha1_test

import (
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestPreviewPolicyRejectsUnsupportedProfilesAndUnboundedLimits(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy v1.PreviewPolicy
		valid  bool
	}{
		{"default", v1.PreviewPolicy{}, true},
		{"disabled", v1.PreviewPolicy{Profile: "disabled"}, true},
		{"persistent", v1.PreviewPolicy{Profile: "postgresql"}, false},
		{"short", v1.PreviewPolicy{TTLSeconds: 1}, false},
		{"long", v1.PreviewPolicy{TTLSeconds: 604801}, false},
		{"capacity", v1.PreviewPolicy{MaxActive: 51}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, err := test.policy.Normalize()
			if (err == nil) != test.valid {
				t.Fatal(policy, err)
			}
			if test.name == "default" && (policy.Profile != "stateless-v1" || policy.TTLSeconds != 86400 || policy.MaxActive != 10) {
				t.Fatal("default policy changed", policy)
			}
		})
	}
}
