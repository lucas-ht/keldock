//go:build integration

package access_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func identityDatabase(t *testing.T) (*postgres.Store, *access.Repository, context.Context) {
	t.Helper()
	data, err := os.ReadFile("../../.local/runtime.json")
	if err != nil {
		t.Fatal("run task local:up first")
	}
	var config struct {
		DatabaseURL string `json:"databaseUrl"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(config.DatabaseURL)
	if err != nil || endpoint.Host != "127.0.0.1:55432" || endpoint.Path != "/keldock" {
		t.Fatal("refusing identity tests outside the disposable keldock-m1 database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	store, err := postgres.Open(ctx, config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	organization := "org-" + strings.TrimPrefix(access.NewID("id"), "id-")
	for index, project := range []string{"sample", "restricted"} {
		if err := store.SeedProject(ctx, v1.Project{Scope: v1.Scope{OrganizationID: organization, ProjectID: project}, Name: project, RepositoryID: int64(42 + index), ClusterID: "keldock-m1"}); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := access.NewRepository(store.Pool(), organization)
	if err != nil {
		t.Fatal(err)
	}
	return store, repository, ctx
}

func administrator(t *testing.T, repository *access.Repository, ctx context.Context) (access.Principal, string, string) {
	t.Helper()
	invitation, err := repository.Bootstrap(ctx, "admin@example.test", "Local administrator")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("a private local test password")
	if err != nil {
		t.Fatal(err)
	}
	user, session, err := repository.Activate(ctx, invitation.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	principal, _, err := repository.Session(ctx, session)
	if err != nil || principal.User.ID != user.ID || !principal.IsAdmin() {
		t.Fatal("administrator activation failed")
	}
	return principal, session, hash
}
