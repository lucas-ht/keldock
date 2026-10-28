package bootstrap

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func testSpec() Spec {
	return Spec{Scope: v1.Scope{OrganizationID: "test-org", ProjectID: "example"}, InstallationID: "test", ClusterID: "test-cluster", Account: "123456789012", Region: "eu-central-1", ParentZoneID: "Z12345", ParentDomain: "example.com", Domain: "test.example.com", IngressIPv4: "127.0.0.1", Development: true}
}

func TestApprovalCoversEveryTargetAndPolicy(t *testing.T) {
	plan, err := Build(testSpec())
	if err != nil {
		t.Fatal(err)
	}
	before, err := plan.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var copyPlan Plan
	if err := json.Unmarshal(data, &copyPlan); err != nil {
		t.Fatal(err)
	}
	after, err := copyPlan.Fingerprint()
	if err != nil || before != after {
		t.Fatalf("JSON roundtrip changed approval: %v", err)
	}
	copyPlan.Identity["Description"] = "changed"
	changed, _ := copyPlan.Fingerprint()
	if changed == before {
		t.Fatal("template change retained old approval")
	}
	copyPlan = plan
	copyPlan.Spec.ParentZoneID = "ZOTHER"
	changed, _ = copyPlan.Fingerprint()
	if changed == before {
		t.Fatal("parent target change retained old approval")
	}
}

func TestBootstrapRejectsApexAndUnboundedTargets(t *testing.T) {
	for name, mutate := range map[string]func(*Spec){
		"apex":            func(s *Spec) { s.Domain = s.ParentDomain },
		"suffix trick":    func(s *Spec) { s.Domain = "otherexample.com" },
		"public suffix":   func(s *Spec) { s.ParentDomain = "com" },
		"foreign account": func(s *Spec) { s.Account = "*" },
		"bad region":      func(s *Spec) { s.Region = "*" },
		"zone traversal":  func(s *Spec) { s.ParentZoneID = "Z123/*" },
		"public loopback": func(s *Spec) { s.Development = false },
		"long identity":   func(s *Spec) { s.InstallationID = "installation-too-long" },
	} {
		t.Run(name, func(t *testing.T) {
			s := testSpec()
			mutate(&s)
			if _, err := Build(s); err == nil {
				t.Fatal("accepted invalid bootstrap target")
			}
		})
	}
}

func TestRetainedTrustRootsAndSingleParentDelegation(t *testing.T) {
	spec := testSpec()
	plan, err := Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range []map[string]any{plan.Foundation, plan.Identity} {
		for name, raw := range template["Resources"].(map[string]any) {
			resource := raw.(map[string]any)
			if resource["DeletionPolicy"] != "Retain" || resource["UpdateReplacePolicy"] != "Retain" {
				t.Fatalf("%s may destroy retained data or trust roots", name)
			}
		}
	}
	resources := plan.Foundation["Resources"].(map[string]any)
	delegation := resources["ParentDelegation"].(map[string]any)["Properties"].(map[string]any)
	if delegation["Name"] != spec.Domain || delegation["HostedZoneId"] != spec.ParentZoneID || delegation["Type"] != "NS" {
		t.Fatal("parent zone delegation escaped exact child target")
	}
	if strings.HasPrefix(spec.IssuerBucket(), spec.DataPrefix()) {
		t.Fatal("provider can manage issuer bucket")
	}
}
