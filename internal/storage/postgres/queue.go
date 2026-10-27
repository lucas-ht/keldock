package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Work is a leased publication operation with an immutable admitted input snapshot.
type Work struct {
	Operation  v1.Operation
	Input      v1.SubmitRelease
	Selection  compiler.Selection
	LeaseToken string
}

// Claim leases one pending/expired publication without blocking other consumers.
func (store *Store) Claim(ctx context.Context, organization string, duration time.Duration) (*Work, error) {
	if !v1.ValidName(organization) || duration < time.Second || duration > 5*time.Minute {
		return nil, ErrConflict
	}
	var work Work
	var input, selection []byte
	work.LeaseToken = newID("lease")
	err := store.pool.QueryRow(ctx, `WITH candidate AS (
 SELECT o.id FROM operations o JOIN environments e ON e.id=o.environment_id
 WHERE o.kind='deploy' AND e.organization_id=$3 AND o.generation=e.generation AND NOT e.deleted AND o.available_at<=now()
 AND (o.state='pending' OR (o.state='publishing' AND o.lease_until<now()))
 ORDER BY o.available_at,o.id FOR UPDATE OF o SKIP LOCKED LIMIT 1
), leased AS (
 UPDATE operations o SET state='publishing',attempt=attempt+1,lease_token=$1,
 lease_until=now()+make_interval(secs => $2),updated_at=now()
 FROM candidate c WHERE o.id=c.id RETURNING o.*
)
SELECT o.id,o.environment_id,o.deployment_id,o.generation,o.attempt,e.organization_id,e.project_id,r.input,d.selection
FROM leased o JOIN environments e ON e.id=o.environment_id JOIN deployments d ON d.id=o.deployment_id JOIN releases r ON r.id=d.release_id`, work.LeaseToken, duration.Seconds(), organization).Scan(
		&work.Operation.ID, &work.Operation.EnvironmentID, &work.Operation.DeploymentID, &work.Operation.Generation, &work.Operation.Attempt,
		&work.Operation.OrganizationID, &work.Operation.ProjectID, &input, &selection)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(input, &work.Input); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(selection, &work.Selection); err != nil {
		return nil, err
	}
	work.Operation.Kind, work.Operation.State = "deploy", "publishing"
	return &work, nil
}

// Published accepts a worker result only while its exact lease/generation is current.
// A rejected lease can leave a harmless content-addressed artifact, never a new selection.
func (store *Store) Published(ctx context.Context, work Work, bundle string) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var current int64
	var deleted bool
	if err := tx.QueryRow(ctx, `SELECT generation,deleted FROM environments WHERE id=$1 FOR UPDATE`, work.Operation.EnvironmentID).Scan(&current, &deleted); err != nil {
		return err
	}
	if deleted || current != work.Operation.Generation {
		return ErrStale
	}
	result, err := tx.Exec(ctx, `UPDATE operations SET state='reconciling',lease_token='',lease_until=NULL,error='',updated_at=now() WHERE id=$1 AND lease_token=$2 AND lease_until>now() AND state='publishing'`, work.Operation.ID, work.LeaseToken)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrStale
	}
	if _, err := tx.Exec(ctx, `UPDATE deployments SET bundle=$2 WHERE id=$1`, work.Operation.DeploymentID, bundle); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Failed records a bounded error and schedules a finite retry only for the current lease.
func (store *Store) Failed(ctx context.Context, work Work, message string) error {
	if len(message) > 2048 {
		message = message[:2048]
	}
	state := "pending"
	if work.Operation.Attempt >= 3 {
		state = "failed"
	}
	result, err := store.pool.Exec(ctx, `UPDATE operations o SET state=$3,error=$4,lease_token='',lease_until=NULL,available_at=now()+interval '2 seconds',updated_at=now()
FROM environments e WHERE o.id=$1 AND o.lease_token=$2 AND o.lease_until>now() AND o.state='publishing' AND o.environment_id=e.id AND o.generation=e.generation AND NOT e.deleted`, work.Operation.ID, work.LeaseToken, state, message)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrStale
	}
	return nil
}

// Cancel freezes future dispatch and leaves already completed external effects visible.
// It is deliberately not an automatic data or release rollback.
func (store *Store) Cancel(ctx context.Context, scope v1.Scope, operationID, actor string) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var environment string
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT e.id,e.generation FROM environments e JOIN operations o ON o.environment_id=e.id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3 AND o.generation=e.generation FOR UPDATE OF e`, operationID, scope.OrganizationID, scope.ProjectID).Scan(&environment, &generation); err != nil {
		return ErrNotFound
	}
	result, err := tx.Exec(ctx, `UPDATE operations SET state='canceled',lease_token='',lease_until=NULL,updated_at=now() WHERE id=$1 AND state IN ('pending','publishing','reconciling','failed')`, operationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,project_id,actor,event,target,generation) VALUES ($1,$2,$3,'operation.canceled',$4,$5)`, scope.OrganizationID, scope.ProjectID, actor, environment, generation); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
