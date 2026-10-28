package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// CancelRevision freezes an exact nonproduction operation; completed external effects remain.
func (store *Store) CancelRevision(ctx context.Context, scope v1.Scope, actor string, request v1.OperationMutation, authorize MutationAuthority) (v1.Receipt, error) {
	var receipt v1.Receipt
	if authorize == nil || request.Generation < 1 {
		return receipt, ErrDenied
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer rollback(ctx, tx)
	if err = authorize(ctx, tx); err != nil {
		return receipt, err
	}
	intent := struct {
		Action  string
		Request v1.OperationMutation
	}{"cancel", request}
	receipt, replay, err := mutationReceipt(ctx, tx, scope, actor, request.RequestKey, intent)
	if err != nil {
		return receipt, err
	}
	if replay {
		return receipt, tx.Commit(ctx)
	}
	var current int64
	var kind string
	var deleted bool
	err = tx.QueryRow(ctx, `SELECT e.generation,e.kind,e.deleted,e.id,coalesce(d.id,''),coalesce(d.release_id,''),o.generation FROM environments e JOIN operations o ON o.environment_id=e.id LEFT JOIN deployments d ON d.id=o.deployment_id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3 FOR UPDATE OF e,o`, request.OperationID, scope.OrganizationID, scope.ProjectID).Scan(&current, &kind, &deleted, &receipt.EnvironmentID, &receipt.DeploymentID, &receipt.ReleaseID, &receipt.Generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, ErrNotFound
	}
	if err != nil {
		return receipt, err
	}
	if kind != "staging" && kind != "preview" {
		return receipt, ErrDenied
	}
	if deleted || current != request.Generation || current != receipt.Generation {
		return receipt, ErrStale
	}
	result, err := tx.Exec(ctx, `UPDATE operations SET state='canceled',lease_token='',lease_until=NULL,updated_at=now() WHERE id=$1 AND kind='deploy' AND state IN ('pending','publishing','reconciling','failed')`, request.OperationID)
	if err != nil {
		return receipt, err
	}
	if result.RowsAffected() != 1 {
		return receipt, ErrConflict
	}
	receipt.APIVersion = v1.APIVersion
	receipt.OperationID = request.OperationID
	receipt.StatusURL = "/v1/projects/" + scope.ProjectID + "/operations/" + receipt.OperationID
	if err = saveMutation(ctx, tx, scope, actor, request.RequestKey, intent, receipt, "operation.canceled"); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}
