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

// Desired is one approved environment projection for its assigned outbound cluster agent.
type Desired = v1.ProjectedDeployment

// Operation retrieves a receipt's live outcome without permitting cross-project lookup.
func (store *Store) Operation(ctx context.Context, scope v1.Scope, id string) (v1.Operation, error) {
	var operation v1.Operation
	operation.Scope = scope
	err := store.pool.QueryRow(ctx, `SELECT o.id,coalesce(o.deployment_id,''),o.environment_id,o.generation,o.kind,o.state,o.attempt,o.error,o.updated_at
FROM operations o JOIN environments e ON e.id=o.environment_id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3`, id, scope.OrganizationID, scope.ProjectID).Scan(
		&operation.ID, &operation.DeploymentID, &operation.EnvironmentID, &operation.Generation, &operation.Kind, &operation.State, &operation.Attempt, &operation.Error, &operation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return operation, ErrNotFound
	}
	return operation, err
}

// Desired returns a bounded, ordered snapshot for one authenticated organization/cluster.
func (store *Store) Desired(ctx context.Context, organization, cluster string) ([]Desired, error) {
	items, _, err := store.DesiredPage(ctx, organization, cluster, "")
	return items, err
}

// DesiredPage uses stable keyset pagination and bounded five-environment responses.
func (store *Store) DesiredPage(ctx context.Context, organization, cluster, after string) ([]Desired, string, error) {
	return store.desiredPage(ctx, organization, cluster, after, "")
}

func (store *Store) desiredPage(ctx context.Context, organization, cluster, after, environment string) ([]Desired, string, error) {
	rows, err := store.pool.Query(ctx, `SELECT o.id,e.organization_id,e.project_id,o.environment_id,coalesce(o.deployment_id,''),o.generation,o.kind,o.state,o.attempt,
coalesce(d.selection,'{}'::jsonb),coalesce(r.input,'{}'::jsonb),coalesce(d.bundle,'')
FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation
LEFT JOIN deployments d ON d.id=o.deployment_id LEFT JOIN releases r ON r.id=d.release_id
WHERE e.organization_id=$1 AND e.cluster_id=$2 AND e.id>$3 AND ($4='' OR e.id=$4) AND o.state IN ('reconciling','ready','failed','canceled') ORDER BY e.id LIMIT 6`, organization, cluster, after, environment)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	result := []Desired{}
	for rows.Next() {
		var desired Desired
		var selection, input []byte
		err := rows.Scan(&desired.Operation.ID, &desired.Operation.OrganizationID, &desired.Operation.ProjectID, &desired.Operation.EnvironmentID, &desired.Operation.DeploymentID, &desired.Operation.Generation, &desired.Operation.Kind, &desired.Operation.State, &desired.Operation.Attempt, &selection, &input, &desired.Bundle)
		if err != nil {
			return nil, "", err
		}
		if desired.Operation.Kind == "delete" {
			desired.Selection = compiler.Selection{EnvironmentID: desired.Operation.EnvironmentID, Generation: desired.Operation.Generation, Namespace: "ke-" + desired.Operation.EnvironmentID[4:]}
		} else {
			var release v1.SubmitRelease
			if err := json.Unmarshal(selection, &desired.Selection); err != nil {
				return nil, "", err
			}
			if err := json.Unmarshal(input, &release); err != nil {
				return nil, "", err
			}
			desired.Definition = release.Definition
			desired.Images = release.Images
		}
		result = append(result, desired)
	}
	var next string
	if len(result) > 5 {
		result = result[:5]
		next = result[4].Operation.EnvironmentID
	}
	return result, next, rows.Err()
}

// CurrentDesired rechecks a listed environment against current approved selection.
func (store *Store) CurrentDesired(ctx context.Context, organization, cluster, environment string) (*Desired, error) {
	if !v1.ValidName(environment) {
		return nil, ErrNotFound
	}
	items, _, err := store.desiredPage(ctx, organization, cluster, "", environment)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Operation.EnvironmentID == environment {
			return &item, nil
		}
	}
	return nil, ErrNotFound
}

// Observe accepts current revision evidence from a scoped cluster identity.
func (store *Store) Observe(ctx context.Context, organization, cluster string, report v1.Observation) error {
	if report.State != "ready" && report.State != "reconciling" && report.State != "failed" && report.State != "deleted" {
		return ErrConflict
	}
	if len(report.Components) > 16 || report.ObservedAt.After(time.Now().Add(time.Minute)) || report.ObservedAt.Before(time.Now().Add(-5*time.Minute)) {
		return ErrConflict
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var current int64
	var operation, state, kind, deployment, bundle string
	var input []byte
	err = tx.QueryRow(ctx, `SELECT e.generation,o.id,o.state,o.kind,coalesce(o.deployment_id,''),coalesce(d.bundle,''),coalesce(r.input,'{}'::jsonb) FROM environments e
JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation LEFT JOIN deployments d ON d.id=o.deployment_id
LEFT JOIN releases r ON r.id=d.release_id
WHERE e.id=$1 AND e.organization_id=$2 AND e.cluster_id=$3 FOR UPDATE OF e,o`, report.EnvironmentID, organization, cluster).Scan(&current, &operation, &state, &kind, &deployment, &bundle, &input)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != report.Generation || deployment != report.DeploymentID || bundle != report.Bundle || state == "canceled" || state == "pending" || state == "publishing" || (kind != "delete" && report.State == "deleted") || (kind == "delete" && report.State == "ready") {
		return ErrStale
	}
	var release v1.SubmitRelease
	if err := json.Unmarshal(input, &release); err != nil {
		return err
	}
	if err := report.Validate(release.Definition); err != nil {
		return ErrConflict
	}
	if report.State == "ready" {
		for _, component := range report.Components {
			if component.Image != release.Images[component.Name] {
				return ErrConflict
			}
		}
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO observations VALUES ($1,$2,$3,now()) ON CONFLICT (environment_id) DO UPDATE SET generation=EXCLUDED.generation,report=EXCLUDED.report,received_at=now()
WHERE EXCLUDED.generation>observations.generation OR (EXCLUDED.generation=observations.generation AND
((EXCLUDED.report->>'observedAt')::timestamptz>(observations.report->>'observedAt')::timestamptz OR EXCLUDED.report=observations.report))`, report.EnvironmentID, report.Generation, data)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrStale
	}
	if _, err := tx.Exec(ctx, `UPDATE operations SET state=$2,updated_at=now(),error='' WHERE id=$1`, operation, report.State); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LatestObservation exposes only evidence matching the requested operation and project.
func (store *Store) LatestObservation(ctx context.Context, scope v1.Scope, operationID string) (*v1.Observation, error) {
	var data []byte
	err := store.pool.QueryRow(ctx, `SELECT v.report FROM observations v JOIN environments e ON e.id=v.environment_id JOIN operations o ON o.environment_id=e.id AND o.generation=v.generation
WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3`, operationID, scope.OrganizationID, scope.ProjectID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var report v1.Observation
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}
