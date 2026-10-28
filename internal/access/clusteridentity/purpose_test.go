package clusteridentity

import (
	"strings"
	"testing"
)

func TestCloudIdentityUsesSeparateAuthority(t *testing.T) {
	query := "UPDATE access_agents FROM access_clusters JOIN access_enrollments"
	workload := &Repository{}
	cloud := &Repository{cloud: true}
	if workload.statement(query) != query {
		t.Fatal("default identity table changed")
	}
	for _, table := range []string{"access_cloud_agents", "access_cloud_clusters", "access_cloud_enrollments"} {
		if !strings.Contains(cloud.statement(query), table) {
			t.Fatalf("missing separate table %s", table)
		}
	}
}
