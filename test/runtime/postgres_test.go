//go:build integration

package runtime_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func scopedInput(t *testing.T, store *postgres.Store, ctx context.Context) (v1.Scope, v1.SubmitRelease) {
	t.Helper()
	scope := v1.Scope{OrganizationID: "org-" + strings.ToLower(rand.Text()), ProjectID: "sample"}
	project := v1.Project{Scope: scope, Name: "Sample", RepositoryID: 42, ClusterID: "keldock-m1"}
	if err := store.SeedProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: scope.ProjectID,
		Definition: v1.Application{APIVersion: v1.APIVersion, Name: "sample", Components: map[string]v1.Component{"web": {Kind: "http", Size: "small", Port: 8080, Probe: &v1.Probe{Path: "/ready"}}}},
		Source:     v1.Source{RepositoryID: 42, OwnerID: 24, Repository: "example/sample", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main", Event: "push", WorkflowRef: "example/sample/.github/workflows/build.yaml@refs/heads/main", RunID: 1, RunAttempt: 1},
		Images:     map[string]string{"web": "registry.example/sample@sha256:" + strings.Repeat("b", 64)}, Provenance: json.RawMessage(`{}`)}
	return scope, input
}

func TestDurableReceiptsAndExpiredLeases(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	var group sync.WaitGroup
	receipts := make(chan v1.Receipt, 12)
	errorsFound := make(chan error, 12)
	for range 12 {
		group.Go(func() {
			receipt, err := store.Admit(ctx, scope, "ci", "same-request", "sample.localhost", input)
			if err != nil {
				errorsFound <- err
				return
			}
			receipts <- receipt
		})
	}
	group.Wait()
	close(receipts)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	var original v1.Receipt
	for receipt := range receipts {
		if original.OperationID != "" && receipt != original {
			t.Fatal("duplicate acceptance created different work")
		}
		original = receipt
	}
	if original.OperationID == "" {
		t.Fatal("no durable receipt")
	}
	first, err := store.Claim(ctx, scope.OrganizationID, time.Second)
	if err != nil || first == nil {
		t.Fatalf("claim: %v", err)
	}
	if duplicate, err := store.Claim(ctx, scope.OrganizationID, time.Second); err != nil || duplicate != nil {
		t.Fatalf("leased work claimed twice: %v", err)
	}
	store.Close()
	restarted, err := postgres.Open(ctx, configuration(t).DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	time.Sleep(1100 * time.Millisecond)
	second, err := restarted.Claim(ctx, scope.OrganizationID, time.Minute)
	if err != nil || second == nil || second.Operation.ID != first.Operation.ID || second.Operation.Attempt != 2 {
		t.Fatalf("expired lease not recovered: %+v %v", second, err)
	}
	bundle := "registry.example/bundles@sha256:" + strings.Repeat("c", 64)
	if err := restarted.Published(ctx, *first, bundle); !errors.Is(err, postgres.ErrStale) {
		t.Fatalf("old lease completed: %v", err)
	}
	if err := restarted.Published(ctx, *second, bundle); err != nil {
		t.Fatal(err)
	}
	state, err := restarted.Operation(ctx, scope, original.OperationID)
	if err != nil || state.State != "reconciling" {
		t.Fatalf("dispatch outcome lost: %+v %v", state, err)
	}
	wrong := scope
	wrong.ProjectID = "other"
	if _, err := restarted.Operation(ctx, wrong, original.OperationID); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("cross-project lookup: %v", err)
	}
	if _, err := restarted.Admit(ctx, wrong, "ci", "wrong-project", "sample.localhost", input); !errors.Is(err, postgres.ErrDenied) {
		t.Fatalf("cross-project acceptance: %v", err)
	}
	input.Source.RunID++
	if _, err := restarted.Admit(ctx, scope, "ci", "same-request", "sample.localhost", input); !errors.Is(err, postgres.ErrConflict) {
		t.Fatalf("idempotency input changed: %v", err)
	}
}

func TestSupersessionAndCancellation(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	first, err := store.Admit(ctx, scope, "ci", "first-request", "sample.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	work, err := store.Claim(ctx, scope.OrganizationID, time.Minute)
	if err != nil || work == nil {
		t.Fatalf("claim: %v", err)
	}
	input.Source.RunID++
	second, err := store.Admit(ctx, scope, "ci", "second-request", "sample.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != first.Generation+1 {
		t.Fatal("selection did not advance")
	}
	if err := store.Published(ctx, *work, "old-bundle"); !errors.Is(err, postgres.ErrStale) {
		t.Fatalf("superseded publication won: %v", err)
	}
	state, err := store.Operation(ctx, scope, first.OperationID)
	if err != nil || state.State != "superseded" {
		t.Fatalf("old work not superseded: %v", err)
	}
	if err := store.Cancel(ctx, scope, second.OperationID, "operator"); err != nil {
		t.Fatal(err)
	}
	if work, err := store.Claim(ctx, scope.OrganizationID, time.Minute); err != nil || work != nil {
		t.Fatalf("canceled work dispatched: %v", err)
	}
}

func TestPreviewClosureAndWebhookReplay(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	input.Source.Event = "pull_request"
	input.Source.PullRequest = 7
	input.Source.HeadCommit = input.Source.Commit
	input.Source.Ref = "refs/pull/7/merge"
	update := postgres.PullRequestUpdate{RepositoryID: 42, Number: 7, HeadCommit: input.Source.HeadCommit, HeadRepositoryID: 42, UpdatedAt: time.Now().UTC()}
	delivery := scope.OrganizationID + "/"
	if changed, err := store.RecordPullRequest(ctx, scope, delivery+"delivery-open", "hash-open", update); err != nil || !changed {
		t.Fatalf("open: %v", err)
	}
	if changed, err := store.RecordPullRequest(ctx, scope, delivery+"delivery-open", "hash-open", update); err != nil || changed {
		t.Fatalf("replay: %v", err)
	}
	if _, err := store.RecordPullRequest(ctx, scope, delivery+"delivery-open", "changed-hash", update); !errors.Is(err, postgres.ErrConflict) {
		t.Fatalf("conflicting delivery accepted: %v", err)
	}
	receipt, err := store.Admit(ctx, scope, "ci", "preview-request", "preview.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	update.Closed = true
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	if changed, err := store.RecordPullRequest(ctx, scope, delivery+"delivery-close", "hash-close", update); err != nil || !changed {
		t.Fatalf("close: %v", err)
	}
	update.Closed = false
	update.UpdatedAt = update.UpdatedAt.Add(-time.Second)
	if changed, err := store.RecordPullRequest(ctx, scope, delivery+"delivery-stale", "hash-stale", update); err != nil || changed {
		t.Fatalf("stale event reopened preview: %v", err)
	}
	input.Source.RunID++
	if _, err := store.Admit(ctx, scope, "ci", "late-ci-request", "preview.localhost", input); !errors.Is(err, postgres.ErrStale) {
		t.Fatalf("late CI resurrected preview: %v", err)
	}
	desired, err := store.Desired(ctx, scope.OrganizationID, "keldock-m1")
	if err != nil || len(desired) != 1 || desired[0].Operation.Kind != "delete" || desired[0].Operation.Generation != receipt.Generation+1 {
		t.Fatalf("no durable teardown: %+v %v", desired, err)
	}
}
