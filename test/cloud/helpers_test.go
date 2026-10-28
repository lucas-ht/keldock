//go:build integration

package cloud_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/bootstrap"
	cloudstore "github.com/lucas-ht/keldock/internal/cloud/store"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func database(t *testing.T) (context.Context, *postgres.Store, *cloudstore.Store, access.Principal) {
	t.Helper()
	data, err := os.ReadFile("../../.local/runtime.json")
	if err != nil {
		t.Fatal("run task local:up first")
	}
	var configuration struct {
		DatabaseURL string `json:"databaseUrl"`
	}
	if err := json.Unmarshal(data, &configuration); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(configuration.DatabaseURL)
	if err != nil || u.Host != "127.0.0.1:55432" || u.Path != "/keldock" {
		t.Fatal("refusing cloud database tests outside disposable local target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	workloads, err := postgres.Open(ctx, configuration.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workloads.Close)
	if err := workloads.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	scope := v1.Scope{OrganizationID: access.NewID("org"), ProjectID: "sample"}
	if err := workloads.SeedProject(ctx, v1.Project{Scope: scope, Name: "Cloud fixture", RepositoryID: 42, ClusterID: "keldock-m1"}); err != nil {
		t.Fatal(err)
	}
	users, err := access.NewRepository(workloads.Pool(), scope.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := users.Bootstrap(ctx, "admin@example.test", "Cloud operator")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("local cloud fixture password")
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := users.Activate(ctx, grant.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	principal, _, err := users.Session(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	policy := (bootstrap.Spec{Scope: scope, InstallationID: "fixture", ClusterID: "keldock-m1", Account: "123456789012", Region: "eu-central-1", ParentZoneID: "ZPARENT", ParentDomain: "example.test", Domain: "cloud.example.test", IngressIPv4: "127.0.0.1", Development: true}).Policy("ZCHILD")
	policy.EgressCIDRs = []string{"3.120.1.2/32"}
	return ctx, workloads, &cloudstore.Store{Pool: workloads.Pool(), Users: users, Policies: map[string]cloud.Policy{"sample": policy}}, principal
}

func command(t *testing.T, kind, id string, value any) cloud.Command {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return cloud.Command{Kind: kind, ID: id, Input: data}
}

func verifyConnection(t *testing.T, ctx context.Context, store *cloudstore.Store, principal access.Principal) cloud.Receipt {
	t.Helper()
	request := cloud.ConnectionRequest{Provider: "aws", Account: "123456789012", Region: "eu-central-1", Profiles: []string{"private-storage", "backup-storage", "public-domain"}, IdentityRef: "aws-default", Generation: 1}
	receipt, err := store.Mutate(ctx, principal, "sample", "connection-fixture", command(t, "connection", "aws-fixture", request))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", cloud.Evidence{ID: receipt.ID, Generation: 1, State: "ready", Reason: "SyntheticIdentityFixture", Details: map[string]string{"account": request.Account}, ObservedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	return receipt
}
