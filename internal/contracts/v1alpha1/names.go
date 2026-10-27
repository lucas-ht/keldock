package v1alpha1

import (
	"crypto/sha256"
	"fmt"
)

// EnvironmentID keeps one stable target for an application and PR (zero means staging).
func EnvironmentID(scope Scope, application string, pullRequest int64) string {
	digest := sha256.Sum256(fmt.Appendf(nil, "%s/%s/%s/%d", scope.OrganizationID, scope.ProjectID, application, pullRequest))
	return "env-" + fmt.Sprintf("%x", digest)[:24]
}
