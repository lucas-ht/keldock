//go:build integration

package runtime_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	dbcap "github.com/lucas-ht/keldock/internal/database"
	databasestore "github.com/lucas-ht/keldock/internal/database/store"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func TestPostgreSQLAllocationIsolationAndRetention(t *testing.T) {
	s, ctx := database(t)
	scope, input := scopedInput(t, s, ctx)
	publish := func() {
		t.Helper()
		work, err := s.Claim(ctx, scope.OrganizationID, time.Minute)
		if err != nil || work == nil {
			t.Fatalf("publication dispatch: %v", err)
		}
		if err := s.Published(ctx, *work, "registry.example/bundles@sha256:"+strings.Repeat("c", 64)); err != nil {
			t.Fatal(err)
		}
	}
	input.Definition.Resources = map[string]v1.ResourceRequest{"data": {Kind: "postgresql", Profile: "development"}}
	c := input.Definition.Components["web"]
	c.Resources = []string{"data"}
	input.Definition.Components["web"] = c
	if _, err := s.Admit(ctx, scope, "ci", "without-delegation", "sample.localhost", input); !errors.Is(err, postgres.ErrDenied) {
		t.Fatalf("uninstalled capability allocated: %v", err)
	}
	p := dbcap.Policy{Scope: scope, ClusterID: "keldock-m1", Limit: 2, Profiles: map[string]dbcap.Profile{"development": {StorageClass: "local-path", StorageSize: "1Gi"}}}
	if err := s.SeedDatabasePolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	staging, err := s.Admit(ctx, scope, "ci", "staging-with-data", "sample.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	publish()
	first, err := s.CurrentDesired(ctx, scope.OrganizationID, "keldock-m1", staging.EnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	input.Source.Event = "pull_request"
	input.Source.PullRequest = 16
	input.Source.HeadCommit = input.Source.Commit
	input.Source.Ref = "refs/pull/16/merge"
	update := postgres.PullRequestUpdate{RepositoryID: 42, Number: 16, HeadCommit: input.Source.HeadCommit, HeadRepositoryID: 42, UpdatedAt: time.Now().UTC()}
	if _, err := s.RecordPullRequest(ctx, scope, scope.OrganizationID+"-open", "open-hash", update); err != nil {
		t.Fatal(err)
	}
	preview, err := s.Admit(ctx, scope, "ci", "preview-with-data", "preview.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	publish()
	second, err := s.CurrentDesired(ctx, scope.OrganizationID, "keldock-m1", preview.EnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Selection.Databases["data"].Namespace == second.Selection.Databases["data"].Namespace {
		t.Fatal("preview shares staging database")
	}
	update.Closed = true
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	if _, err := s.RecordPullRequest(ctx, scope, scope.OrganizationID+"-closed", "closed-hash", update); err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := s.Pool().QueryRow(ctx, `SELECT intent FROM database_allocations WHERE environment_id=$1`, preview.EnvironmentID).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var retained dbcap.Allocation
	if err := json.Unmarshal(data, &retained); err != nil {
		t.Fatal(err)
	}
	if retained.Action != "retain" || retained.Generation != 2 || len(retained.Components) != 0 {
		t.Fatal("preview cleanup did not retain data and tombstone consumers")
	}
	users, err := access.NewRepository(s.Pool(), scope.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := users.Bootstrap(ctx, "admin@example.test", "Database operator")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("synthetic database operator password")
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := users.Activate(ctx, grant.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	principal, _, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	store := databasestore.Store{Pool: s.Pool(), Users: users, Policies: map[string]dbcap.Policy{scope.ProjectID: p}}
	items, err := store.List(ctx, principal, scope.ProjectID)
	if err != nil || len(items) != 2 {
		t.Fatalf("retained inventory: %v", err)
	}
	command := databasestore.Command{ID: first.Selection.Databases["data"].ResourceID, Generation: 1, Action: "retain"}
	result, err := store.Mutate(ctx, principal, scope.ProjectID, "retain-allocation-key", command)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := store.Mutate(ctx, principal, scope.ProjectID, "retain-allocation-key", command); err != nil || replay.Generation != result.Generation {
		t.Fatalf("lost lifecycle response replay: %v", err)
	}
	publish()
	current, err := s.CurrentDesired(ctx, scope.OrganizationID, "keldock-m1", staging.EnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Selection.Databases) != 0 || current.Selection.Generation != 2 || current.Images["web"] != first.Images["web"] {
		t.Fatal("disconnect did not remove binding while preserving admitted images")
	}
	wrong := principal
	wrong.User.OrganizationID = "other"
	if _, err := store.List(ctx, wrong, scope.ProjectID); !errors.Is(err, access.ErrDenied) {
		t.Fatal("cross-organization database inventory exposed")
	}
	if err := store.Observe(ctx, scope.OrganizationID, "keldock-m1", dbcap.Evidence{Scope: scope, ID: command.ID, Generation: 1, State: "ready", Reason: "stale", ObservedAt: time.Now().UTC()}); !errors.Is(err, postgres.ErrStale) {
		t.Fatalf("stale database health accepted: %v", err)
	}
	command.Generation = result.Generation
	command.Action = "reconnect"
	if _, err := store.Mutate(ctx, principal, scope.ProjectID, strings.Repeat("r", 20), command); err != nil {
		t.Fatal(err)
	}
}
