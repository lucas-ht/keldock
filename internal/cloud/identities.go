package cloud

import (
	"crypto/sha256"
	"fmt"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// ShortIdentity gives generated Kubernetes resource names a stable bounded suffix.
func ShortIdentity(id string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:20] }

// WorkloadIdentity is stable for one approved component, allowing policy union across its stores.
func WorkloadIdentity(connection Connection, profile Profile, environment, component string) (string, string) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{connection.OrganizationID, connection.ProjectID, connection.ID, environment, component}, "/"))))[:24]
	return "wk-" + hash, "arn:aws:iam::" + connection.Account + ":role" + profile.RolePath + ConsumerRolePrefix(profile) + hash
}

// ConsumerRolePrefix permits narrow IAM name-only lookups before a role/path exists.
func ConsumerRolePrefix(profile Profile) string {
	return "kc-" + ShortIdentity(profile.RolePath)[:12] + "-"
}

// BackupKey isolates Kubernetes projections and source ownership across projects.
func BackupKey(scope v1.Scope, id string) string {
	return ShortIdentity(scope.OrganizationID + "/" + scope.ProjectID + "/" + id)
}

// ArchivePrefix isolates retained backup histories even on a shared supplied bucket.
func ArchivePrefix(scope v1.Scope, id string, generation int64) string {
	return fmt.Sprintf("organizations/%s/projects/%s/archives/%s/generation-%d", scope.OrganizationID, scope.ProjectID, strings.TrimPrefix(id, "cloud-"), generation)
}
