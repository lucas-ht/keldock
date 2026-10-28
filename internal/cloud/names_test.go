package cloud

import (
	"strings"
	"testing"
)

func TestBucketNamesAreStableAndScopedToAccountRegionAndResource(t *testing.T) {
	policy, connection := fixturePolicy()
	first, err := policy.BucketName(connection, "res-one", "private-storage")
	if err != nil {
		t.Fatal(err)
	}
	same, err := policy.BucketName(connection, "res-one", "private-storage")
	if err != nil || first != same {
		t.Fatal("unstable resource identity", err)
	}
	if len(first) > 63 || !strings.HasPrefix(first, "keldock-test-") || !strings.HasSuffix(first, "-123456789012-eu-central-1-an") {
		t.Fatal("name escaped account-regional namespace")
	}
	other, err := policy.BucketName(connection, "res-two", "private-storage")
	if err != nil || first == other {
		t.Fatal("resource identities collided", err)
	}
	for _, region := range []string{"us-east-1", "ap-southeast-2", "eu-central-1"} {
		policy.Regions = []string{region}
		connection.Region = region
		name, err := policy.BucketName(connection, "res-one", "private-storage")
		if err != nil || !strings.HasSuffix(name, "-123456789012-"+region+"-an") || len(name) > 63 {
			t.Fatal("invalid regional namespace", err)
		}
	}
	if _, err := policy.BucketName(connection, "res-one", "public-domain"); err == nil {
		t.Fatal("domain profile generated storage")
	}
}
