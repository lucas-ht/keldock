package backup

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lucas-ht/keldock/internal/database"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func archivedWAL(ctx context.Context, client *s3.Client, a database.Allocation, destination database.Destination, backup *unstructured.Unstructured) (bool, *time.Time, error) {
	prefix := database.ArchivePrefix(a.Scope, a.ID) + "/database/wals/"
	end, _, _ := unstructured.NestedString(backup.Object, "status", "endWal")
	var start *string
	if len(end) == 24 && strings.Trim(end, "0123456789ABCDEF") == "" {
		start = aws.String(prefix + end[:16] + "/" + end)
	}
	var latest *time.Time
	seen := false
	page, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(destination.Bucket), Prefix: aws.String(prefix), StartAfter: start, MaxKeys: aws.Int32(1000)})
	if err != nil {
		return false, nil, err
	}
	for _, object := range page.Contents {
		if object.Key == nil || object.Size == nil || *object.Size < 1 {
			continue
		}
		key := strings.TrimSuffix(*object.Key, ".gz")
		part := key[strings.LastIndex(key, "/")+1:]
		if len(part) != 24 || strings.Trim(part, "0123456789ABCDEF") != "" {
			continue
		}
		seen = true
		if object.LastModified != nil && (latest == nil || object.LastModified.After(*latest)) {
			copy := *object.LastModified
			latest = &copy
		}
	}
	return seen, latest, nil
}
