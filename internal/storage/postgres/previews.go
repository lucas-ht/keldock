package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// PullRequestUpdate is authenticated GitHub state; event time prevents stale reopening.
type PullRequestUpdate struct {
	RepositoryID     int64
	Number           int64
	HeadCommit       string
	HeadRepositoryID int64
	Closed           bool
	UpdatedAt        time.Time
}

// RecordPullRequest durably deduplicates an authenticated delivery and updates previews.
// A closed state wins timestamp ties; a strictly newer reopen may permit new admission.
func (store *Store) RecordPullRequest(ctx context.Context, scope v1.Scope, delivery, payloadHash string, update PullRequestUpdate) (bool, error) {
	if len(delivery) < 8 || len(delivery) > 128 || update.Number <= 0 || update.HeadRepositoryID <= 0 || update.UpdatedAt.IsZero() {
		return false, ErrConflict
	}
	if decoded, err := hex.DecodeString(update.HeadCommit); err != nil || len(decoded) != 20 {
		return false, ErrConflict
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(ctx, tx)
	var repository int64
	if err := tx.QueryRow(ctx, `SELECT repository_id FROM projects WHERE organization_id=$1 AND id=$2 FOR UPDATE`, scope.OrganizationID, scope.ProjectID).Scan(&repository); err != nil || repository != update.RepositoryID {
		return false, ErrDenied
	}
	result, err := tx.Exec(ctx, `INSERT INTO webhook_deliveries VALUES ($1,$2,$3,$4,now()) ON CONFLICT DO NOTHING`, delivery, scope.OrganizationID, scope.ProjectID, payloadHash)
	if err != nil {
		return false, err
	}
	if result.RowsAffected() == 0 {
		var original string
		if err := tx.QueryRow(ctx, `SELECT payload_hash FROM webhook_deliveries WHERE id=$1 AND organization_id=$2 AND project_id=$3`, delivery, scope.OrganizationID, scope.ProjectID).Scan(&original); err != nil || original != payloadHash {
			return false, ErrConflict
		}
		return false, tx.Commit(ctx)
	}
	var previouslyClosed bool
	previous := tx.QueryRow(ctx, `SELECT closed FROM pull_requests WHERE organization_id=$1 AND project_id=$2 AND number=$3 FOR UPDATE`, scope.OrganizationID, scope.ProjectID, update.Number).Scan(&previouslyClosed)
	if previous != nil && !errors.Is(previous, pgx.ErrNoRows) {
		return false, previous
	}
	result, err = tx.Exec(ctx, `INSERT INTO pull_requests VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (organization_id,project_id,number) DO UPDATE SET head_commit=EXCLUDED.head_commit,head_repository_id=EXCLUDED.head_repository_id,closed=EXCLUDED.closed,updated_at=EXCLUDED.updated_at
WHERE EXCLUDED.updated_at>pull_requests.updated_at OR (EXCLUDED.updated_at=pull_requests.updated_at AND EXCLUDED.closed AND NOT pull_requests.closed)`, scope.OrganizationID, scope.ProjectID, update.Number, update.HeadCommit, update.HeadRepositoryID, update.Closed, update.UpdatedAt)
	if err != nil {
		return false, err
	}
	if result.RowsAffected() == 0 {
		return false, tx.Commit(ctx)
	}
	if update.Closed {
		if err := closePreviews(ctx, tx, scope, update.Number); err != nil {
			return false, err
		}
	} else if previouslyClosed {
		if _, err := tx.Exec(ctx, `UPDATE environments SET deleted=false,expires_at=NULL,deletion_reason='' WHERE organization_id=$1 AND project_id=$2 AND pull_request=$3 AND deleted`, scope.OrganizationID, scope.ProjectID, update.Number); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,project_id,actor,event,target,generation) VALUES ($1,$2,'github','pull_request.updated',$3,0)`, scope.OrganizationID, scope.ProjectID, delivery); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func closePreviews(ctx context.Context, tx pgx.Tx, scope v1.Scope, number int64) error {
	rows, err := tx.Query(ctx, `SELECT id,generation FROM environments WHERE organization_id=$1 AND project_id=$2 AND pull_request=$3 AND NOT deleted ORDER BY id FOR UPDATE`, scope.OrganizationID, scope.ProjectID, number)
	if err != nil {
		return err
	}
	type target struct {
		id         string
		generation int64
	}
	var targets []target
	for rows.Next() {
		var value target
		if err := rows.Scan(&value.id, &value.generation); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, target := range targets {
		if err := deleteSelection(ctx, tx, target.id, target.generation+1, "pull_request_closed"); err != nil {
			return err
		}
	}
	return nil
}

func deleteSelection(ctx context.Context, tx pgx.Tx, environment string, generation int64, reason string) error {
	if err := detachCloudConsumers(ctx, tx, environment); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE environments SET deleted=true,generation=$2,deletion_reason=$3 WHERE id=$1`, environment, generation, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE operations SET state='superseded',lease_token='',lease_until=NULL,updated_at=now() WHERE environment_id=$1 AND state NOT IN ('deleted','superseded')`, environment); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO operations (id,environment_id,generation,kind,state) VALUES ($1,$2,$3,'delete','reconciling')`, newID("op"), environment, generation); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(organization_id,project_id,actor,event,target,generation) SELECT organization_id,project_id,'system:preview-policy',$3,id,$2 FROM environments WHERE id=$1`, environment, generation, "preview."+reason)
	return err
}

// ExpirePreviews creates deletion intent without relying on a CI cleanup workflow.
func (store *Store) ExpirePreviews(ctx context.Context, organization string) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var id string
	var project string
	var generation int64
	err = tx.QueryRow(ctx, `SELECT id,project_id FROM environments WHERE organization_id=$1 AND pull_request>0 AND expires_at<=now() AND NOT deleted ORDER BY id LIMIT 1`, organization).Scan(&id, &project)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organization, project).Scan(&locked); err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT generation FROM environments WHERE id=$1 AND NOT deleted AND expires_at<=now() FOR UPDATE SKIP LOCKED`, id).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := deleteSelection(ctx, tx, id, generation+1, "expired"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
