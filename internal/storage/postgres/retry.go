package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Retry reuses the selected admitted release. Product callers supply transactional authority;
// the optional form is reserved for already-authorized internal persistence tests.
func (store *Store) Retry(ctx context.Context, scope v1.Scope, actor, id string, generation int64, key string, authority ...MutationAuthority) (v1.Receipt, error) {
	var receipt v1.Receipt
	if generation < 1 || len(authority) > 1 {
		return receipt, ErrConflict
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer rollback(ctx, tx)
	if len(authority) == 1 {
		if authority[0] == nil {
			return receipt, ErrDenied
		}
		if err = authority[0](ctx, tx); err != nil {
			return receipt, err
		}
	}
	request := v1.OperationMutation{OperationID: id, Generation: generation, RequestKey: key}
	intent := struct {
		Action  string
		Request v1.OperationMutation
	}{"retry", request}
	receipt, replay, err := mutationReceipt(ctx, tx, scope, actor, key, intent)
	if err != nil {
		return receipt, err
	}
	if replay {
		return receipt, tx.Commit(ctx)
	}
	policy, err := mutationPolicy(ctx, tx, scope)
	if err != nil {
		return receipt, err
	}
	var environment, release, state, operationKind, kind, sourceHead string
	var current, pull int64
	var deleted, fresh bool
	var configuration []byte
	err = tx.QueryRow(ctx, `SELECT e.id,e.generation,e.deleted,o.state,o.kind,coalesce(d.release_id,''),coalesce(d.selection,'{}'::jsonb),e.kind,e.pull_request,coalesce(r.input->'source'->>'headCommit',''),coalesce(e.expires_at>clock_timestamp(),true) FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation LEFT JOIN deployments d ON d.id=o.deployment_id LEFT JOIN releases r ON r.id=d.release_id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3 FOR UPDATE OF e,o`, id, scope.OrganizationID, scope.ProjectID).Scan(&environment, &current, &deleted, &state, &operationKind, &release, &configuration, &kind, &pull, &sourceHead, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, ErrStale
	}
	if err != nil {
		return receipt, err
	}
	if kind != "preview" && kind != "staging" {
		return receipt, ErrDenied
	}
	if deleted || !fresh || current != generation {
		return receipt, ErrStale
	}
	if operationKind != "deploy" || (state != "failed" && state != "canceled") {
		return receipt, ErrConflict
	}
	if pull > 0 {
		if policy.Profile == "disabled" {
			return receipt, ErrDenied
		}
		var allowed bool
		if err = tx.QueryRow(ctx, `SELECT NOT closed AND head_commit=$4 AND head_repository_id=(SELECT repository_id FROM projects WHERE organization_id=$1 AND id=$2) FROM pull_requests WHERE organization_id=$1 AND project_id=$2 AND number=$3 FOR SHARE`, scope.OrganizationID, scope.ProjectID, pull, sourceHead).Scan(&allowed); err != nil || !allowed {
			return receipt, ErrStale
		}
	}
	var selection v1.Placement
	if err = json.Unmarshal(configuration, &selection); err != nil {
		return receipt, err
	}
	selection.Generation = current + 1
	selection.DeploymentID = newID("dep")
	receipt, err = selectDeployment(ctx, tx, scope, release, selection)
	if err != nil {
		return receipt, err
	}
	if _, err = tx.Exec(ctx, `UPDATE environments SET generation=$2 WHERE id=$1`, environment, selection.Generation); err != nil {
		return receipt, err
	}
	if err = saveMutation(ctx, tx, scope, actor, key, intent, receipt, "operation.retried"); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}
