//go:build integration

package runtime_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func TestPreviewPolicyStableIdentityCapacityAndExpiryFence(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	project := v1.Project{Scope: scope, Name: "Sample", RepositoryID: 42, ClusterID: "keldock-m1", Previews: v1.PreviewPolicy{Profile: "stateless-v1", TTLSeconds: 60, MaxActive: 1}}
	if err := store.SeedProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	update := postgres.PullRequestUpdate{RepositoryID: 42, Number: 61, HeadCommit: strings.Repeat("a", 40), HeadRepositoryID: 42, UpdatedAt: now}
	record := func(delivery string, change postgres.PullRequestUpdate) {
		t.Helper()
		if _, err := store.RecordPullRequest(ctx, scope, scope.OrganizationID+"-"+delivery, "hash-"+delivery, change); err != nil {
			t.Fatal(err)
		}
	}
	record("preview-policy-open-61", update)
	input.Source.Event = "pull_request"
	input.Source.Ref = "refs/pull/61/merge"
	input.Source.WorkflowRef = "example/sample/.github/workflows/build.yaml@refs/pull/61/merge"
	input.Source.PullRequest = 61
	input.Source.HeadCommit = update.HeadCommit
	first, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-first", "operator.test", input)
	if err != nil {
		t.Fatal(err)
	}
	input.Definition.Name = "renamed"
	input.Source.RunID++
	next, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-renamed", "operator.test", input)
	if err != nil || next.EnvironmentID != first.EnvironmentID || next.Generation != 2 {
		t.Fatal("app rename changed PR identity", err)
	}
	var host string
	var ttl float64
	if err := store.Pool().QueryRow(ctx, `SELECT d.selection->>'hostname',extract(epoch FROM e.expires_at-now()) FROM environments e JOIN deployments d ON d.environment_id=e.id AND d.generation=e.generation WHERE e.id=$1`, first.EnvironmentID).Scan(&host, &ttl); err != nil {
		t.Fatal(err)
	}
	if host != "preview-"+strings.TrimPrefix(first.EnvironmentID, "env-")[:12]+".operator.test" || ttl < 45 || ttl > 60 {
		t.Fatal("operator domain/TTL lost", host, ttl)
	}
	second := input
	second.Source.PullRequest = 62
	second.Source.Ref = "refs/pull/62/merge"
	second.Source.WorkflowRef = "example/sample/.github/workflows/build.yaml@refs/pull/62/merge"
	other := update
	other.Number = 62
	record("preview-policy-open-62", other)
	if _, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-over-quota", "operator.test", second); !errors.Is(err, postgres.ErrQuota) {
		t.Fatal("preview capacity not enforced", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE environments SET expires_at=now()-interval '1 second' WHERE id=$1`, first.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	if err := store.ExpirePreviews(ctx, scope.OrganizationID); err != nil {
		t.Fatal(err)
	}
	update.UpdatedAt = now.Add(time.Minute)
	update.HeadCommit = strings.Repeat("b", 40)
	record("preview-policy-late-open", update)
	input.Source.HeadCommit = update.HeadCommit
	input.Source.RunID++
	if _, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-expired-ci", "operator.test", input); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("late CI resurrected expiry", err)
	}
	if _, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-cleanup-reserved", "operator.test", second); !errors.Is(err, postgres.ErrQuota) {
		t.Fatal("disconnected cleanup released capacity early", err)
	}
	// This database-policy test marks cleanup complete; real pruning is covered by the runtime lifecycle.
	if _, err := store.Pool().Exec(ctx, `UPDATE operations SET state='deleted' WHERE environment_id=$1 AND kind='delete'`, first.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-new-capacity", "operator.test", second); err != nil {
		t.Fatal(err)
	}
	update.Closed = true
	update.UpdatedAt = now.Add(2 * time.Minute)
	record("preview-policy-closed-61", update)
	update.Closed = false
	update.UpdatedAt = now.Add(3 * time.Minute)
	record("preview-policy-reopened-61", update)
	if _, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-reopen-quota", "operator.test", input); !errors.Is(err, postgres.ErrQuota) {
		t.Fatal(fmt.Sprintf("reopen escaped quota: %v", err))
	}
	project.Previews.Profile = "disabled"
	if err := store.SeedProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitToDomain(ctx, scope, "ci", "preview-policy-disabled", "operator.test", input); !errors.Is(err, postgres.ErrDenied) {
		t.Fatal("disabled profile admitted preview", err)
	}
}

func TestPreviewCapacitySerializesCompetingAdmissions(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	if err := store.SeedProject(ctx, v1.Project{Scope: scope, Name: "Sample", RepositoryID: 42, ClusterID: "keldock-m1", Previews: v1.PreviewPolicy{MaxActive: 1}}); err != nil {
		t.Fatal(err)
	}
	requests := make([]v1.SubmitRelease, 2)
	for i := range requests {
		pull := int64(80 + i)
		requests[i] = input
		requests[i].Source.PullRequest = pull
		requests[i].Source.HeadCommit = input.Source.Commit
		requests[i].Source.Event = "pull_request"
		requests[i].Source.Ref = fmt.Sprintf("refs/pull/%d/merge", pull)
		requests[i].Source.WorkflowRef = "example/sample/.github/workflows/build.yaml@" + requests[i].Source.Ref
		if _, err := store.RecordPullRequest(ctx, scope, scope.OrganizationID+fmt.Sprintf("-capacity-open-%d", pull), "hash", postgres.PullRequestUpdate{RepositoryID: 42, Number: pull, HeadRepositoryID: 42, HeadCommit: input.Source.Commit, UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 2)
	for i, input := range requests {
		go func() {
			_, err := store.AdmitToDomain(ctx, scope, "ci", fmt.Sprintf("capacity-request-%d", i), "operator.test", input)
			results <- err
		}()
	}
	accepted, denied := 0, 0
	for range requests {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, postgres.ErrQuota):
			denied++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 1 || denied != 1 {
		t.Fatal("competing admissions exceeded capacity", accepted, denied)
	}
}

func TestPreviewRetryRejectsChangedHeadAndExpiredIntent(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	input.Source.PullRequest = 97
	input.Source.Event = "pull_request"
	input.Source.HeadCommit = input.Source.Commit
	input.Source.Ref = "refs/pull/97/merge"
	input.Source.WorkflowRef = "example/sample/.github/workflows/build.yaml@" + input.Source.Ref
	update := postgres.PullRequestUpdate{RepositoryID: 42, Number: 97, HeadRepositoryID: 42, HeadCommit: input.Source.HeadCommit, UpdatedAt: time.Now().UTC()}
	record := func(label string) {
		t.Helper()
		if _, err := store.RecordPullRequest(ctx, scope, scope.OrganizationID+"-"+label, "hash", update); err != nil {
			t.Fatal(err)
		}
	}
	record("retry-open")
	receipt, err := store.AdmitToDomain(ctx, scope, "ci", "retry-preview-original", "operator.test", input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE operations SET state='failed' WHERE id=$1`, receipt.OperationID); err != nil {
		t.Fatal(err)
	}
	update.HeadCommit = strings.Repeat("c", 40)
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	record("retry-changed-head")
	if _, err = store.Retry(ctx, scope, "operator", receipt.OperationID, receipt.Generation, "retry-stale-head"); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("retry accepted an obsolete PR head", err)
	}
	update.HeadCommit = input.Source.HeadCommit
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	record("retry-restored-head")
	update.HeadRepositoryID = 99
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	record("retry-foreign-head")
	if _, err = store.Retry(ctx, scope, "operator", receipt.OperationID, receipt.Generation, "retry-foreign-repository"); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("retry accepted foreign head repository", err)
	}
	update.HeadRepositoryID = 42
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	record("retry-restored-repository")
	if _, err = store.Pool().Exec(ctx, `UPDATE environments SET expires_at=now()-interval '1 second' WHERE id=$1`, receipt.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Retry(ctx, scope, "operator", receipt.OperationID, receipt.Generation, "retry-expired-head"); !errors.Is(err, postgres.ErrStale) {
		t.Fatal("retry extended an expired preview", err)
	}
}
