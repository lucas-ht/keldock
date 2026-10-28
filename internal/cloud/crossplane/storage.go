package crossplane

import (
	"fmt"
	"slices"

	"github.com/lucas-ht/keldock/internal/cloud"
)

// Storage allocates metadata only. Buckets and all independent controls omit Delete.
func Storage(policy cloud.Policy, connection cloud.Connection, resource cloud.Resource) ([]map[string]any, error) {
	if err := policy.ValidateConnection(connection); err != nil {
		return nil, err
	}
	profile, ok := policy.Profiles[resource.Profile]
	if !ok || resource.Scope != policy.Scope || resource.ConnectionID != connection.ID || resource.ConnectionGeneration != connection.Generation || !slices.Contains(connection.Profiles, resource.Profile) || profile.Kind != resource.Kind || resource.Retention != "retain" {
		return nil, fmt.Errorf("storage projection outside approved connection")
	}
	observe := resource.Owner != "crossplane"
	parameters := map[string]any{"region": connection.Region, "forceDestroy": false}
	if resource.Mode == "create" {
		expected, err := policy.BucketName(connection, resource.ID, resource.Profile)
		if err != nil || expected != resource.ExternalName {
			return nil, fmt.Errorf("managed bucket identity differs")
		}
		parameters["bucketNamespace"] = "account-regional"
	} else if !slices.Contains(profile.ExistingBuckets, resource.ExternalName) {
		return nil, fmt.Errorf("existing bucket lacks an operator grant")
	}
	bucket := managed(policy, connection, resource.ID, "Bucket", "s3.aws.m.upbound.io", "s3", resource.ExternalName, parameters, observe)
	result := []map[string]any{bucket}
	if observe {
		return result, nil
	}
	// The bucket object deliberately owns no tags, lifecycle, bucket policy, ACL or replication.
	// Independent resources own only this profile's selected security/versioning fields.
	controls := map[string]map[string]any{
		"BucketVersioning":                        {"versioningConfiguration": map[string]string{"status": "Enabled"}},
		"BucketPublicAccessBlock":                 {"blockPublicAcls": true, "ignorePublicAcls": true, "blockPublicPolicy": true, "restrictPublicBuckets": true},
		"BucketOwnershipControls":                 {"rule": map[string]string{"objectOwnership": "BucketOwnerEnforced"}},
		"BucketServerSideEncryptionConfiguration": {"rule": []any{map[string]any{"applyServerSideEncryptionByDefault": map[string]string{"sseAlgorithm": "AES256"}, "bucketKeyEnabled": false}}},
	}
	for _, kind := range []string{"BucketVersioning", "BucketPublicAccessBlock", "BucketOwnershipControls", "BucketServerSideEncryptionConfiguration"} {
		parameters := controls[kind]
		parameters["region"] = connection.Region
		parameters["bucket"] = resource.ExternalName
		if kind == "BucketVersioning" || kind == "BucketServerSideEncryptionConfiguration" {
			parameters["expectedBucketOwner"] = connection.Account
		}
		result = append(result, managed(policy, connection, resource.ID, kind, "s3.aws.m.upbound.io", "s3", resource.ExternalName, parameters, false))
	}
	return result, nil
}
