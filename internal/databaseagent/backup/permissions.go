package backup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/lucas-ht/keldock/internal/database"
)

func accessDenied(err error) bool {
	var api smithy.APIError
	return errors.As(err, &api) && (api.ErrorCode() == "AccessDenied" || api.ErrorCode() == "Forbidden")
}

func probeBackupPermissions(ctx context.Context, writer, reader *s3.Client, a database.Allocation, destination database.Destination) (result error) {
	prefix := database.ArchivePrefix(a.Scope, a.ID) + "/"
	for _, client := range []*s3.Client{writer, reader} {
		if _, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(destination.Bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1)}); err != nil {
			return fmt.Errorf("backup identity cannot list its archive")
		}
		if _, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(destination.Bucket), Prefix: aws.String("outside-keldock-allocation/"), MaxKeys: aws.Int32(1)}); !accessDenied(err) {
			return fmt.Errorf("backup identity is not restricted to its archive")
		}
	}
	key := prefix + ".keldock-permission-check"
	if _, err := writer.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(destination.Bucket), Key: aws.String(key), Body: strings.NewReader("Keldock scoped backup check")}); err != nil {
		return fmt.Errorf("backup writer cannot write its archive")
	}
	// Always remove both exact synthetic keys, including an unexpectedly permitted
	// reader write. Neither archive objects nor other prefixes are deleted.
	defer func() {
		for _, target := range []string{key, key + "-reader"} {
			if _, err := writer.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(destination.Bucket), Key: aws.String(target)}); err != nil {
				result = errors.Join(result, fmt.Errorf("backup permission check could not be cleaned up"))
			}
		}
	}()
	object, err := reader.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(destination.Bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("restore identity cannot read its archive")
	}
	if err := object.Body.Close(); err != nil {
		return fmt.Errorf("restore access check interrupted")
	}
	if _, err := reader.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(destination.Bucket), Key: aws.String(key + "-reader"), Body: strings.NewReader("read-only identity must deny this")}); !accessDenied(err) {
		return fmt.Errorf("restore identity is not read-only")
	}
	return nil
}
