package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
)

// Retry selects the same admitted release in a new generation; it cannot introduce CI inputs.
// The caller must authorize a current human operator for scope before entering this boundary.
func (store *Store) Retry(ctx context.Context, scope v1.Scope, actor, id string, generation int64, key string) (v1.Receipt, error) {
	var receipt v1.Receipt
	if len(key) < 8 || len(key) > 128 || generation < 1 || actor == "" {
		return receipt, ErrConflict
	}
	fingerprint, err := provenance.Hash(struct {
		Operation  string
		Generation int64
	}{id, generation})
	if err != nil {
		return receipt, err
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer rollback(ctx, tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, scope.OrganizationID+"/"+scope.ProjectID+"/"+key); err != nil {
		return receipt, err
	}
	var savedHash string
	var saved []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response FROM receipts WHERE organization_id=$1 AND project_id=$2 AND key=$3`, scope.OrganizationID, scope.ProjectID, key).Scan(&savedHash, &saved)
	if err == nil {
		if savedHash != fingerprint {
			return receipt, ErrConflict
		}
		if err := json.Unmarshal(saved, &receipt); err != nil {
			return receipt, err
		}
		return receipt, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	var pr int64
	err = tx.QueryRow(ctx, `SELECT e.pull_request FROM environments e JOIN operations o ON o.environment_id=e.id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3`, id, scope.OrganizationID, scope.ProjectID).Scan(&pr)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, ErrNotFound
	}
	if err != nil {
		return receipt, err
	}
	if pr > 0 {
		var closed bool
		if err := tx.QueryRow(ctx, `SELECT closed FROM pull_requests WHERE organization_id=$1 AND project_id=$2 AND number=$3 FOR SHARE`, scope.OrganizationID, scope.ProjectID, pr).Scan(&closed); err != nil {
			return receipt, err
		}
		if closed {
			return receipt, ErrStale
		}
	}
	var environment, release, state, kind string
	var current int64
	var deleted bool
	var configuration []byte
	err = tx.QueryRow(ctx, `SELECT e.id,e.generation,e.deleted,o.state,o.kind,coalesce(d.release_id,''),coalesce(d.selection,'{}'::jsonb) FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation LEFT JOIN deployments d ON d.id=o.deployment_id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3 FOR UPDATE OF e,o`, id, scope.OrganizationID, scope.ProjectID).Scan(&environment, &current, &deleted, &state, &kind, &release, &configuration)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, ErrStale
	}
	if err != nil {
		return receipt, err
	}
	if deleted || current != generation || kind != "deploy" || (state != "failed" && state != "canceled") {
		return receipt, ErrConflict
	}
	var selection v1.Placement
	if err := json.Unmarshal(configuration, &selection); err != nil {
		return receipt, err
	}
	operation, deployment := newID("op"), newID("dep")
	selection.Generation = current + 1
	selection.DeploymentID = deployment
	data, err := json.Marshal(selection)
	if err != nil {
		return receipt, err
	}
	hash, err := provenance.Hash(selection)
	if err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO deployments (id,environment_id,release_id,generation,config_hash,selection) VALUES ($1,$2,$3,$4,$5,$6)`, deployment, environment, release, selection.Generation, hash, data); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `UPDATE operations SET state='superseded',lease_token='',lease_until=NULL,updated_at=now() WHERE id=$1`, id); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `UPDATE environments SET generation=$2 WHERE id=$1`, environment, selection.Generation); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO operations (id,environment_id,deployment_id,generation,kind,state) VALUES ($1,$2,$3,$4,'deploy','pending')`, operation, environment, deployment, selection.Generation); err != nil {
		return receipt, err
	}
	receipt = v1.Receipt{APIVersion: v1.APIVersion, ReleaseID: release, EnvironmentID: environment, DeploymentID: deployment, OperationID: operation, Generation: selection.Generation, StatusURL: "/v1/projects/" + scope.ProjectID + "/operations/" + operation}
	response, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO receipts VALUES ($1,$2,$3,$4,$5)`, scope.OrganizationID, scope.ProjectID, key, fingerprint, response); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,project_id,actor,event,target,generation) VALUES ($1,$2,$3,'operation.retried',$4,$5)`, scope.OrganizationID, scope.ProjectID, actor, environment, selection.Generation); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}
