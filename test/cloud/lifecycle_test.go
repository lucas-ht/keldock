//go:build integration

package cloud_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func TestCloudReceiptsAuthorityAndRetainedInventory(t *testing.T) {
	ctx, _, store, principal := database(t)
	connection := verifyConnection(t, ctx, store, principal)
	request := command(t, "resource", "", cloud.ResourceRequest{ConnectionID: connection.ID, Kind: cloud.ObjectStorage, Profile: "private-storage", Mode: "create"})
	first, err := store.Mutate(ctx, principal, "sample", "resource-request-1", request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Mutate(ctx, principal, "sample", "resource-request-1", request)
	if err != nil || first != second {
		t.Fatal("lost response did not recover original receipt", err)
	}
	request.Input = []byte(`{"connectionId":"aws-fixture","kind":"object-storage","profile":"private-storage","mode":"connect","existingName":"unapproved-bucket"}`)
	if _, err := store.Mutate(ctx, principal, "sample", "resource-request-1", request); !errors.Is(err, postgres.ErrConflict) {
		t.Fatal("receipt allowed changed intent")
	}
	if _, err := store.Mutate(ctx, principal, "sample", "unapproved-connect", request); !errors.Is(err, access.ErrDenied) {
		t.Fatal("unapproved existing bucket admitted", err)
	}
	mcp := principal
	mcp.Kind = "mcp"
	mcp.TokenProjects = []string{"sample"}
	mcp.TokenScope = "keldock:read keldock:operate"
	if _, err := store.Mutate(ctx, mcp, "sample", "machine-cannot-admin", request); !errors.Is(err, access.ErrDenied) {
		t.Fatal("M4 grant expanded to cloud administration")
	}
	other := principal
	other.User.OrganizationID = "another-org"
	if _, err := store.List(ctx, other, "sample"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("cross-organization inventory admitted")
	}
	retained, err := store.Mutate(ctx, principal, "sample", "retain-resource-1", command(t, "lifecycle", first.ID, cloud.Mutation{Generation: 1, Action: "retain"}))
	if err != nil || retained.Generation != 2 {
		t.Fatal(err)
	}
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", cloud.Evidence{ProjectID: "sample", ID: first.ID, Generation: 1, ConnectionGeneration: 1, State: "ready", Reason: "OldEvidence", ObservedAt: time.Now().UTC()}); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("stale cloud evidence accepted", err)
	}
	items, err := store.List(ctx, principal, "sample")
	if err != nil || len(items) != 2 {
		t.Fatal("retention removed inventory", err)
	}
	for _, item := range items {
		if item.ID == first.ID && (item.Resource == nil || item.Resource.Retention != "retain" || item.Action != "retain") {
			t.Fatal("retention contract lost")
		}
	}
}

func TestCloudConnectionRevisionAndQuota(t *testing.T) {
	ctx, _, store, principal := database(t)
	connection := verifyConnection(t, ctx, store, principal)
	policy := store.Policies["sample"]
	profile := policy.Profiles["private-storage"]
	profile.MaxResources = 1
	policy.Profiles["private-storage"] = profile
	store.Policies["sample"] = policy
	request := command(t, "resource", "", cloud.ResourceRequest{ConnectionID: connection.ID, Kind: cloud.ObjectStorage, Profile: "private-storage", Mode: "create"})
	if _, err := store.Mutate(ctx, principal, "sample", "first-bounded-resource", request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Mutate(ctx, principal, "sample", "second-bounded-resource", request); !errors.Is(err, postgres.ErrQuota) {
		t.Fatal("cloud capacity unbounded", err)
	}
	if _, err := store.Mutate(ctx, principal, "sample", "suspend-cloud-connection", command(t, "lifecycle", connection.ID, cloud.Mutation{Generation: 1, Action: "suspend"})); err != nil {
		t.Fatal(err)
	}
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", cloud.Evidence{ProjectID: "sample", ID: connection.ID, Generation: 2, State: "ready", Details: map[string]string{"account": "123456789012"}, ObservedAt: time.Now().UTC()}); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("disabled connection became ready")
	}
	if _, err := store.Mutate(ctx, principal, "sample", "disabled-cloud-denial", request); !errors.Is(err, access.ErrDenied) {
		t.Fatal("disabled connection admitted new resource", err)
	}
}
