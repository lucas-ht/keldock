package cloudagent

import (
	"testing"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/bootstrap"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestSnapshotKeepsIdenticalIntentNamesInSeparateProjects(t *testing.T) {
	scope := v1.Scope{OrganizationID: "fixture-org", ProjectID: "first"}
	policy := (bootstrap.Spec{Scope: scope, InstallationID: "fixture", ClusterID: "fixture-cluster", Account: "123456789012", Region: "eu-central-1", ParentZoneID: "ZPARENT", ParentDomain: "example.test", Domain: "test.example.test", IngressIPv4: "127.0.0.1", Development: true}).Policy("ZCHILD")
	other := policy
	other.ProjectID = "second"
	worker := &Agent{Policies: map[string]cloud.Policy{"first": policy, "second": other}}
	first := cloud.Intent{Scope: policy.Scope, ID: "aws-default", Kind: "connection", Generation: 1}
	second := cloud.Intent{Scope: other.Scope, ID: first.ID, Kind: first.Kind, Generation: 2}
	grouped, err := worker.validateSnapshot([]cloud.Intent{first, second})
	if err != nil || grouped["first"][first.ID].Generation != 1 || grouped["second"][second.ID].Generation != 2 {
		t.Fatal("project-local identities collided", err)
	}
	if _, err := worker.validateSnapshot([]cloud.Intent{first, first}); err == nil {
		t.Fatal("duplicate within one project admitted")
	}
	if cloud.BackupKey(first.Scope, "backup") == cloud.BackupKey(second.Scope, "backup") || cloud.ArchivePrefix(first.Scope, "backup", 1) == cloud.ArchivePrefix(second.Scope, "backup", 1) {
		t.Fatal("backup projections or archive data can collide across projects")
	}
}
