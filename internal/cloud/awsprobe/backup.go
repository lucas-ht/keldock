package awsprobe

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lucas-ht/keldock/internal/cloud"
)

// ArchiveWAL verifies metadata under the selected archive generation. Provisioning
// credentials can list keys but cannot read backup contents or restore application data.
func ArchiveWAL(ctx context.Context, configuration aws.Config, connection cloud.Connection, bucket, prefix, server string) (bool, error) {
	result, err := s3.NewFromConfig(configuration).ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), ExpectedBucketOwner: aws.String(connection.Account), Prefix: aws.String(strings.TrimSuffix(prefix, "/") + "/" + server + "/wals/"), MaxKeys: aws.Int32(1)})
	if err != nil {
		return false, err
	}
	return len(result.Contents) > 0, nil
}
