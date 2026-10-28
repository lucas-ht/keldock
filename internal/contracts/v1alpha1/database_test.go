package v1alpha1

import "testing"

func TestDatabaseRequestsCannotSelectInfrastructure(t *testing.T) {
	for _, test := range []struct {
		Name, Input string
		Valid       bool
	}{
		{"supported", `resources: {data: {kind: postgresql, profile: development}}`, true},
		{"secret", `resources: {data: {kind: postgresql, profile: development, secretRef: admin}}`, false},
		{"namespace", `resources: {data: {kind: postgresql, profile: development, namespace: other}}`, false},
		{"external", `resources: {data: {kind: external, profile: development}}`, false},
	} {
		t.Run(test.Name, func(t *testing.T) {
			_, err := DecodeApplication([]byte("apiVersion: keldock.io/v1alpha1\nname: example\n" + test.Input + "\ncomponents:\n  migrate: {kind: job, size: small, resources: [data]}\n"))
			if (err == nil) != test.Valid {
				t.Fatalf("unexpected contract acceptance: %v", err)
			}
		})
	}
}
