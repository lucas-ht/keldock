package cloudagent

import (
	"strings"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/robfig/cron/v3"
)

// Revalidate every privileged target against independent operator configuration.
func validBackup(policy cloud.Policy, conn cloud.Connection, item cloud.Intent, resource cloud.Resource) bool {
	b := item.Backup
	if b == nil || b.ArchiveGeneration < 1 || b.ArchiveGeneration > 16 || int64(len(b.Archives)) != b.ArchiveGeneration || b.BaseAttempt < 0 || b.BaseAttempt > 10 || b.RestoreGeneration < 0 || b.RestoreGeneration > b.ArchiveGeneration || b.RestoreName != "restore-"+cloud.BackupKey(item.Scope, item.ID) {
		return false
	}
	target, ok := policy.Databases[b.DatabaseID]
	if !ok || target != b.Database || resource.Scope != policy.Scope || resource.ConnectionID != conn.ID || resource.Kind != cloud.BackupStore {
		return false
	}
	if _, err := cron.ParseStandard(b.Schedule); err != nil || len(strings.Fields(b.Schedule)) != 5 {
		return false
	}
	profile, ok := policy.Profiles[resource.Profile]
	if !ok {
		return false
	}
	for purpose, actual := range map[string]string{"writer": b.WriterRoleARN, "restore": b.RestoreRoleARN} {
		sa, arn := cloud.WorkloadIdentity(conn, profile, item.ID, purpose)
		// Accept the same deterministic identity from the pre-release fixture naming scheme.
		legacy := "arn:aws:iam::" + conn.Account + ":role" + profile.RolePath + sa
		if actual != arn && actual != legacy {
			return false
		}
	}
	for index, archive := range b.Archives {
		generation := int64(index + 1)
		expected := cloud.ArchivePrefix(item.Scope, item.ID, generation)
		if archive.Generation != generation || archive.Prefix != expected {
			return false
		}
	}
	return b.ArchivePrefix == b.Archives[len(b.Archives)-1].Prefix
}
