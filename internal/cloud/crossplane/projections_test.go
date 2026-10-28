package crossplane

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/bootstrap"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func fixture(t *testing.T) (cloud.Policy, cloud.Connection, cloud.Resource) {
	t.Helper()
	scope := v1.Scope{OrganizationID: "fixture-org", ProjectID: "sample"}
	policy := (bootstrap.Spec{Scope: scope, InstallationID: "fixture", ClusterID: "fixture-cluster", Account: "123456789012", Region: "eu-central-1", ParentZoneID: "ZPARENT", ParentDomain: "example.test", Domain: "cloud.example.test", IngressIPv4: "127.0.0.1", Development: true}).Policy("ZCHILD")
	connection := cloud.Connection{CloudConnection: v1.CloudConnection{Scope: scope, ID: "aws-fixture", Provider: "aws", Account: "123456789012", Region: "eu-central-1", Profiles: []string{"private-storage", "backup-storage", "public-domain"}, IdentityRef: "aws-default"}, ClusterID: policy.ClusterID, Generation: 1, Enabled: true}
	name, err := policy.BucketName(connection, "store-one", "private-storage")
	if err != nil {
		t.Fatal(err)
	}
	resource := cloud.Resource{Resource: v1.Resource{Scope: scope, ID: "store-one", Profile: "private-storage", Generation: 1, Retention: "retain"}, ConnectionID: connection.ID, ConnectionGeneration: 1, Kind: cloud.ObjectStorage, Mode: "create", Owner: "crossplane", ExternalName: name}
	return policy, connection, resource
}

func TestStorageCannotDeleteDataOrTakeUnmanagedFields(t *testing.T) {
	policy, connection, resource := fixture(t)
	objects, err := Storage(policy, connection, resource)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 5 {
		t.Fatal("private storage controls are incomplete")
	}
	for _, object := range objects {
		spec := object["spec"].(map[string]any)
		for _, action := range spec["managementPolicies"].([]string) {
			if action == "Delete" || action == "*" {
				t.Fatal("storage projection may delete data")
			}
		}
		parameters := spec["forProvider"].(map[string]any)
		for _, field := range []string{"tags", "lifecycleRule", "replicationConfiguration", "acl", "policy"} {
			if _, ok := parameters[field]; ok {
				t.Fatalf("projection took unreviewed field %s", field)
			}
		}
	}
	parameters := objects[0]["spec"].(map[string]any)["forProvider"].(map[string]any)
	if parameters["forceDestroy"] != false || parameters["bucketNamespace"] != "account-regional" {
		t.Fatal("storage lacks non-destructive account-regional identity")
	}
	resource.ExternalName = "another-bucket"
	if _, err := Storage(policy, connection, resource); err == nil {
		t.Fatal("managed identity could be caller-chosen")
	}
}

func TestConnectIsObserveOnlyAndCrossProjectIdentityIsDistinct(t *testing.T) {
	policy, connection, resource := fixture(t)
	resource.Mode = "connect"
	resource.Owner = "external"
	resource.ExternalName = policy.Profiles[resource.Profile].ExistingBuckets[0]
	objects, err := Storage(policy, connection, resource)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatal("connect took security configuration ownership")
	}
	actions := objects[0]["spec"].(map[string]any)["managementPolicies"].([]string)
	if len(actions) != 1 || actions[0] != "Observe" {
		t.Fatal("connect can change external storage")
	}
	other := policy.Scope
	other.ProjectID = "other"
	if Name(other, resource.ID, "Bucket") == Name(policy.Scope, resource.ID, "Bucket") {
		t.Fatal("identical IDs collided across projects")
	}
}

func TestConsumerRoleUnionsOnlyBoundedPrefixesAndExactSubject(t *testing.T) {
	policy, connection, _ := fixture(t)
	profile := policy.Profiles["private-storage"]
	sa, arn := cloud.WorkloadIdentity(connection, profile, "env-one", "web")
	role := ConsumerRole{ID: sa, ARN: arn, Namespace: "ke-one", ServiceAccount: sa, Profile: "private-storage", Grants: []Grant{{Bucket: profile.ExistingBuckets[0], Prefix: "projects/sample/one/web/", Access: "read"}, {Bucket: profile.ExistingBuckets[0], Prefix: "projects/sample/two/web/", Access: "read-write"}}}
	object, trust, permissions, err := Role(policy, connection, role)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trust, "system:serviceaccount:ke-one:"+sa) || strings.Contains(trust, "StringLike") {
		t.Fatal("consumer trust is not exact")
	}
	if !strings.Contains(permissions, `"aws:ResourceAccount":"123456789012"`) || strings.Contains(permissions, `"Resource":"*"`) {
		t.Fatal("consumer permissions escaped account or resource authority")
	}
	if object["spec"].(map[string]any)["forProvider"].(map[string]any)["permissionsBoundary"] != profile.BoundaryARN {
		t.Fatal("role can be created without mandatory boundary")
	}
	first, _ := json.Marshal(object)
	role.Grants[0], role.Grants[1] = role.Grants[1], role.Grants[0]
	second, _, _, err := Role(policy, connection, role)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(second)
	if string(first) != string(data) {
		t.Fatal("union order causes perpetual IAM drift")
	}
	role.Grants[0].Prefix = "*"
	if _, _, _, err := Role(policy, connection, role); err == nil {
		t.Fatal("wildcard consumer prefix admitted")
	}
}

func TestRuntimeUsesCommunityDigestsAndSeparateProjectedSubjects(t *testing.T) {
	policy, _, _ := fixture(t)
	objects, err := Runtime(policy)
	if err != nil {
		t.Fatal(err)
	}
	providers := 0
	for _, object := range objects {
		if object["kind"] != "Provider" {
			continue
		}
		providers++
		spec := object["spec"].(map[string]any)
		ref := spec["package"].(string)
		if !strings.HasPrefix(ref, "xpkg.upbound.io/crossplane-contrib/") || !strings.Contains(ref, "@sha256:") || spec["skipDependencyResolution"] != true {
			t.Fatal("provider distribution or transitive pin drift")
		}
	}
	if providers != 4 {
		t.Fatal("fixed AWS provider family is incomplete")
	}
}
