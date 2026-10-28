package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// BucketName resolves a stable account-regional name without accepting a caller's AWS target name.
func (policy Policy) BucketName(connection Connection, resourceID, profileID string) (string, error) {
	if err := policy.ValidateConnection(connection); err != nil {
		return "", err
	}
	profile, ok := policy.Profiles[profileID]
	if !ok || (profile.Kind != ObjectStorage && profile.Kind != BackupStore) || !v1.ValidName(resourceID) {
		return "", fmt.Errorf("storage profile and resource identity required")
	}
	if err := policy.ValidateResource(connection, ResourceRequest{ConnectionID: connection.ID, Kind: profile.Kind, Profile: profileID, Mode: "create"}); err != nil {
		return "", err
	}
	suffix := "-" + connection.Account + "-" + connection.Region + "-an"
	length := min(20, 63-len(profile.BucketPrefix)-len(suffix))
	if length < 12 {
		return "", fmt.Errorf("bucket prefix leaves insufficient space for stable account-regional identity")
	}
	digest := sha256.Sum256(fmt.Appendf(nil, "%s/%s/%s/%s", connection.OrganizationID, connection.ProjectID, connection.ID, resourceID))
	name := profile.BucketPrefix + hex.EncodeToString(digest[:])[:length] + suffix
	if !validBucket(name) {
		return "", fmt.Errorf("generated bucket name is outside the supported profile")
	}
	return name, nil
}
