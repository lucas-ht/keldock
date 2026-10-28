package awsprobe

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/provenance"
)

// BucketSnapshot separates verified identity/security metadata from unproven data access.
type BucketSnapshot struct {
	Account    string            `json:"account"`
	Region     string            `json:"region"`
	Bucket     string            `json:"bucket"`
	Versioning string            `json:"versioning"`
	Private    bool              `json:"private"`
	Encryption string            `json:"encryption"`
	Ownership  string            `json:"ownership"`
	Tags       map[string]string `json:"tags"`
}

// Bucket verifies owner, region and selected properties without reading application objects.
func Bucket(ctx context.Context, configuration aws.Config, connection cloud.Connection, name string) (BucketSnapshot, error) {
	result := BucketSnapshot{Account: connection.Account, Region: connection.Region, Bucket: name, Tags: map[string]string{}}
	client := s3.NewFromConfig(configuration)
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)}); err != nil {
		return result, err
	}
	location, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)})
	if err != nil {
		return result, err
	}
	region := string(location.LocationConstraint)
	if region == "" {
		region = "us-east-1"
	}
	if region == "EU" {
		region = "eu-west-1"
	}
	if region != connection.Region {
		return result, fmt.Errorf("bucket region differs from approved connection")
	}
	versioning, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)})
	if err != nil {
		return result, err
	}
	result.Versioning = string(versioning.Status)
	block, err := client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)})
	if err != nil {
		return result, err
	}
	if b := block.PublicAccessBlockConfiguration; b != nil {
		result.Private = aws.ToBool(b.BlockPublicAcls) && aws.ToBool(b.IgnorePublicAcls) && aws.ToBool(b.BlockPublicPolicy) && aws.ToBool(b.RestrictPublicBuckets)
	}
	encryption, err := client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)})
	if err != nil {
		return result, err
	}
	if encryption.ServerSideEncryptionConfiguration != nil {
		for _, rule := range encryption.ServerSideEncryptionConfiguration.Rules {
			if rule.ApplyServerSideEncryptionByDefault != nil {
				result.Encryption = string(rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
			}
		}
	}
	ownership, err := client.GetBucketOwnershipControls(ctx, &s3.GetBucketOwnershipControlsInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)})
	if err != nil {
		return result, err
	}
	if ownership.OwnershipControls != nil && len(ownership.OwnershipControls.Rules) == 1 {
		result.Ownership = string(ownership.OwnershipControls.Rules[0].ObjectOwnership)
	}
	tags, err := client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(name), ExpectedBucketOwner: aws.String(connection.Account)})
	if err != nil {
		var api smithy.APIError
		if errors.As(err, &api) && api.ErrorCode() == "NoSuchTagSet" {
			return result, nil
		}
		return result, err
	}
	slices.SortFunc(tags.TagSet, func(a, b types.Tag) int {
		if aws.ToString(a.Key) < aws.ToString(b.Key) {
			return -1
		}
		return 1
	})
	for _, tag := range tags.TagSet {
		result.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return result, nil
}

// Hash binds an adoption review to currently observed identity and selected/unmanaged metadata.
func (snapshot BucketSnapshot) Hash() (string, error) { return provenance.Hash(snapshot) }

// UsableProfile verifies the fields delegated by the first private, versioned storage profile.
func (snapshot BucketSnapshot) UsableProfile() bool {
	return snapshot.Private && snapshot.Versioning == "Enabled" && snapshot.Encryption == "AES256" && snapshot.Ownership == "BucketOwnerEnforced"
}
