package cloudagent

import (
	"testing"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/bootstrap"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestConsumerCannotChooseNamespaceRoleOrObjectPrefix(t *testing.T) {
	scope := v1.Scope{OrganizationID: "fixture-org", ProjectID: "sample"}
	policy := (bootstrap.Spec{Scope: scope, InstallationID: "fixture", ClusterID: "fixture-cluster", Account: "123456789012", Region: "eu-central-1", ParentZoneID: "ZPARENT", ParentDomain: "example.test", Domain: "test.example.test", IngressIPv4: "127.0.0.1", Development: true}).Policy("ZCHILD")
	conn := cloud.Connection{CloudConnection: v1.CloudConnection{Scope: scope, ID: "aws-fixture", Account: "123456789012"}, Generation: 1}
	resource := cloud.Resource{Resource: v1.Resource{Scope: scope, Profile: "private-storage"}, ConnectionID: conn.ID}
	binding := cloud.Binding{ResourceBinding: v1.ResourceBinding{Scope: scope, EnvironmentID: "env-one"}}
	sa, arn := cloud.WorkloadIdentity(conn, policy.Profiles[resource.Profile], binding.EnvironmentID, "web")
	consumer := cloud.ConsumerIdentity{Component: "web", Namespace: "ke-one", ServiceAccount: sa, RoleARN: arn, Prefix: "projects/sample/environments/env-one/components/web/"}
	if _, err := consumerRole(policy, conn, binding, consumer, resource); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*cloud.ConsumerIdentity){"namespace": func(c *cloud.ConsumerIdentity) { c.Namespace = "keldock-cloud" }, "service account": func(c *cloud.ConsumerIdentity) { c.ServiceAccount = "provider-iam" }, "role": func(c *cloud.ConsumerIdentity) { c.RoleARN = "arn:aws:iam::123456789012:role/Admin" }, "prefix": func(c *cloud.ConsumerIdentity) { c.Prefix = "*" }} {
		t.Run(name, func(t *testing.T) {
			copy := consumer
			mutate(&copy)
			if _, err := consumerRole(policy, conn, binding, copy, resource); err == nil {
				t.Fatal("untrusted consumer selected privileged placement")
			}
		})
	}
}
