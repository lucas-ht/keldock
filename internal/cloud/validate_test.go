package cloud

import (
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestResourceConnectDoesNotGrantAdoptionOrArbitraryBucketAccess(t *testing.T) {
	tests := []struct {
		name, kind, profile, mode, existing string
		enabled, allowed                    bool
	}{
		{"create", ObjectStorage, "private-storage", "create", "", true, true},
		{"backup", BackupStore, "backup-storage", "create", "", true, true},
		{"connect explicit", ObjectStorage, "private-storage", "connect", "customer-approved-store", true, true},
		{"connect other", ObjectStorage, "private-storage", "connect", "other-private-store", true, false},
		{"create existing", ObjectStorage, "private-storage", "create", "customer-approved-store", true, false},
		{"adopt implicitly", ObjectStorage, "private-storage", "adopt", "customer-approved-store", true, false},
		{"profile mismatch", BackupStore, "private-storage", "create", "", true, false},
		{"disabled", ObjectStorage, "private-storage", "create", "", false, false},
		{"raw provider", "iam-role", "private-storage", "create", "", true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, connection := fixturePolicy()
			connection.Enabled = test.enabled
			request := ResourceRequest{ConnectionID: connection.ID, Kind: test.kind, Profile: test.profile, Mode: test.mode, ExistingName: test.existing}
			if (policy.ValidateResource(connection, request) == nil) != test.allowed {
				t.Fatal("unexpected resource admission")
			}
		})
	}
}

func TestDomainClaimUsesLabelBoundaryAndExactConsumer(t *testing.T) {
	for _, test := range []struct {
		host    string
		allowed bool
	}{
		{"test.example.com", false}, {"preview.test.example.com", true},
		{"not-test.example.com", false}, {"test.example.com.evil.example", false},
		{"*.test.example.com", false}, {"PREVIEW.test.example.com", false}, {"preview.test.example.com.", false},
		{"_acme-challenge.test.example.com", false}, {"127.0.0.1", false},
	} {
		t.Run(test.host, func(t *testing.T) {
			policy, connection := fixturePolicy()
			request := DomainRequest{ConnectionID: connection.ID, Profile: "public-domain", EnvironmentID: "env-one", Component: "web", Hostname: test.host, Generation: 1}
			if (policy.ValidateDomain(connection, request) == nil) != test.allowed {
				t.Fatal("unexpected domain authority")
			}
		})
	}
}

func TestBindingRejectsIdentityAndNamespaceInjection(t *testing.T) {
	good := []byte(`{"resourceId":"res-one","environmentId":"env-one","components":["api"],"access":"read-write","generation":1}`)
	var request BindingRequest
	if err := v1.DecodeJSON(good, &request); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBinding(request); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"roleArn":"arn:aws:iam::123456789012:role/admin"`, `"namespace":"kube-system"`, `"policy":{"Action":"*"}`, `"prefix":"other-project/"`} {
		payload := append(append([]byte{}, good[:len(good)-1]...), []byte(","+field+"}")...)
		if v1.DecodeJSON(payload, &request) == nil {
			t.Fatal("binding accepted caller-controlled authority")
		}
	}
	for _, test := range []struct {
		components []string
		access     string
	}{
		{[]string{"api", "api"}, "read"}, {[]string{"keldock-agent"}, "read"}, {[]string{"api"}, "administrator"}, {nil, "read"},
	} {
		request.Components = test.components
		request.Access = test.access
		if ValidateBinding(request) == nil {
			t.Fatal("invalid consumer grant accepted")
		}
	}
}
