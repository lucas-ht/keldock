package cloud

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// WorkloadIdentity is stable for one approved component, allowing policy union across its stores.
func WorkloadIdentity(connection Connection, profile Profile, environment, component string) (string, string) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{connection.OrganizationID, connection.ProjectID, connection.ID, environment, component}, "/"))))[:24]
	return "wk-" + hash, "arn:aws:iam::" + connection.Account + ":role" + profile.RolePath + "wk-" + hash
}
