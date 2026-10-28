package cloudagent

import (
	"testing"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/bootstrap"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestBackupCannotSelectAnotherWriterArchiveOrRestoreTarget(t *testing.T) {
	scope := v1.Scope{OrganizationID: "fixture-org", ProjectID: "sample"}
	policy := (bootstrap.Spec{Scope: scope, InstallationID: "fixture", ClusterID: "fixture-cluster", Account: "123456789012", Region: "eu-central-1", ParentZoneID: "ZPARENT", ParentDomain: "example.test", Domain: "test.example.test", IngressIPv4: "127.0.0.1", Development: true}).Policy("ZCHILD")
	target := cloud.DatabaseTarget{Namespace: "data", Name: "source", UID: "expected-uid", RestoreNamespace: "sandbox"}
	policy.Databases = map[string]cloud.DatabaseTarget{"source": target}
	conn := cloud.Connection{CloudConnection: v1.CloudConnection{Scope: scope, ID: "aws-fixture", Account: "123456789012"}, Generation: 1}
	resource := cloud.Resource{Resource: v1.Resource{Scope: scope, Profile: "backup-storage"}, ConnectionID: conn.ID, Kind: cloud.BackupStore}
	item := cloud.Intent{Scope: scope, ID: "cloud-test"}
	_, writer := cloud.WorkloadIdentity(conn, policy.Profiles[resource.Profile], item.ID, "writer")
	_, reader := cloud.WorkloadIdentity(conn, policy.Profiles[resource.Profile], item.ID, "restore")
	prefix := cloud.ArchivePrefix(item.Scope, item.ID, 1)
	base := cloud.BackupBinding{DatabaseID: "source", Database: target, WriterRoleARN: writer, RestoreRoleARN: reader, ArchiveGeneration: 1, ArchivePrefix: prefix, Archives: []cloud.ArchiveGeneration{{Generation: 1, Prefix: prefix}}, RestoreName: "restore-" + cloud.BackupKey(item.Scope, item.ID), Schedule: "*/15 * * * *", BaseAttempt: 1}
	item.Backup = &base
	if !validBackup(policy, conn, item, resource) {
		t.Fatal("approved backup rejected")
	}
	for name, mutate := range map[string]func(*cloud.BackupBinding){
		"writer": func(b *cloud.BackupBinding) {
			b.WriterRoleARN = "arn:aws:iam::123456789012:role/keldock/fixture/consumers/another"
		},
		"restore": func(b *cloud.BackupBinding) { b.RestoreName = "production" },
		"source":  func(b *cloud.BackupBinding) { b.Database.UID = "replacement" },
		"archive": func(b *cloud.BackupBinding) { b.ArchivePrefix = "archives/another/generation-1" },
		"history": func(b *cloud.BackupBinding) {
			b.Archives = []cloud.ArchiveGeneration{{Generation: 1, Prefix: "unapproved/"}}
		},
		"schedule": func(b *cloud.BackupBinding) { b.Schedule = "@every 1s" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := base
			mutate(&copy)
			item.Backup = &copy
			if validBackup(policy, conn, item, resource) {
				t.Fatalf("unapproved %s accepted", name)
			}
		})
	}
}
