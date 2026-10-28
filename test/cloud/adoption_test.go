//go:build integration

package cloud_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func TestAdoptionRequiresReleasedWriterCurrentReviewAndConfirmedDisconnect(t *testing.T) {
	ctx, _, store, principal := database(t)
	connection := verifyConnection(t, ctx, store, principal)
	policy := store.Policies["sample"]
	bucket := policy.Profiles["private-storage"].ExistingBuckets[0]
	hash := strings.Repeat("a", 64)
	policy.Adoptions = map[string]cloud.AdoptionGrant{bucket: {InventoryHash: hash, ReleaseRef: "fixture:previous-writer-released", ReleasedAt: time.Now().Add(-time.Minute)}}
	store.Policies["sample"] = policy
	receipt, err := store.Mutate(ctx, principal, "sample", "connect-retained-storage", command(t, "resource", "connected", cloud.ResourceRequest{ConnectionID: connection.ID, Kind: cloud.ObjectStorage, Profile: "private-storage", Mode: "connect", ExistingName: bucket}))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", cloud.Evidence{ProjectID: "sample", ID: receipt.ID, Generation: 1, ConnectionGeneration: 1, State: "ready", Reason: "SyntheticInventory", Details: map[string]string{"inventoryHash": hash}, ObservedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, principal, "sample")
	if err != nil {
		t.Fatal(err)
	}
	review := ""
	for _, item := range items {
		if item.ID == receipt.ID {
			review = item.Observation.Details["adoptionReviewHash"]
		}
	}
	if review == "" {
		t.Fatal("current inventory did not generate adoption review")
	}
	if _, err := store.Mutate(ctx, principal, "sample", "stale-adoption-request", command(t, "lifecycle", receipt.ID, cloud.Mutation{Generation: 1, Action: "adopt", ReviewHash: "old"})); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("stale review admitted", err)
	}
	adopted, err := store.Mutate(ctx, principal, "sample", "current-adoption-request", command(t, "lifecycle", receipt.ID, cloud.Mutation{Generation: 1, Action: "adopt", ReviewHash: review}))
	if err != nil {
		t.Fatal(err)
	}
	disconnected, err := store.Mutate(ctx, principal, "sample", "disconnect-adopted-store", command(t, "lifecycle", receipt.ID, cloud.Mutation{Generation: adopted.Generation, Action: "disconnect"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", cloud.Evidence{ProjectID: "sample", ID: receipt.ID, Generation: disconnected.Generation, ConnectionGeneration: 1, State: "disconnected", Reason: "SyntheticProjectionAbsent", ObservedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	items, err = store.List(ctx, principal, "sample")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == receipt.ID && (item.Resource.Owner != "external" || item.Action != "disconnected") {
			t.Fatal("confirmed disconnect retained writer ownership")
		}
	}
	if _, err := store.Mutate(ctx, principal, "sample", "cannot-revive-disconnected", command(t, "lifecycle", receipt.ID, cloud.Mutation{Generation: disconnected.Generation, Action: "refresh"})); err == nil {
		t.Fatal("disconnected inventory revived without new review")
	}
}
