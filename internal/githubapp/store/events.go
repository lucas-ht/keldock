package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

// Event is durable, bounded webhook work; delivery identity is separate from its lease.
type Event struct {
	ID      string
	Name    string
	Payload json.RawMessage
	Lease   string
	Attempt int
}

// Revocations fence removals before the asynchronous handler can run.
type Revocations struct {
	InstallationID int64
	Installation   bool
	RepositoryIDs  []int64
}

// Receive acknowledges only committed, deduplicated work. Equal delivery IDs require equal bytes.
func (repository *Repository) Receive(ctx context.Context, id, name string, payload []byte, revocations Revocations) (bool, error) {
	if len(id) == 0 || len(id) > 128 || len(name) == 0 || len(name) > 64 || len(payload) > 1024*1024 || !json.Valid(payload) {
		return false, access.ErrInvalid
	}
	hash := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(hash[:])
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(ctx, tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, repository.Organization+"/github/events"); err != nil {
		return false, err
	}
	var previous string
	err = tx.QueryRow(ctx, `SELECT payload_hash FROM github_events WHERE organization_id=$1 AND delivery_id=$2`, repository.Organization, id).Scan(&previous)
	if err == nil {
		if previous != fingerprint {
			return false, access.ErrConflict
		}
		return false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM github_events WHERE organization_id=$1 AND processed_at IS NULL`, repository.Organization).Scan(&pending); err != nil {
		return false, err
	}
	if pending >= 10000 {
		return false, access.ErrLimited
	}
	if _, err := tx.Exec(ctx, `INSERT INTO github_events (organization_id,delivery_id,event,payload_hash,payload) VALUES ($1,$2,$3,$4,$5)`, repository.Organization, id, name, fingerprint, json.RawMessage(payload)); err != nil {
		return false, err
	}
	if revocations.Installation {
		if _, err := tx.Exec(ctx, `UPDATE github_installations SET active=false WHERE organization_id=$1 AND id=$2`, repository.Organization, revocations.InstallationID); err != nil {
			return false, err
		}
	}
	if len(revocations.RepositoryIDs) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE github_repositories SET active=false WHERE organization_id=$1 AND installation_id=$2 AND id=ANY($3)`, repository.Organization, revocations.InstallationID, revocations.RepositoryIDs); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

// ClaimEvent uses an expiring lease so interrupted workers can resume without client state.
func (repository *Repository) ClaimEvent(ctx context.Context) (*Event, error) {
	var event Event
	event.Lease = access.NewID("github-lease")
	err := repository.pool.QueryRow(ctx, `UPDATE github_events SET lease_token=$2,lease_until=now()+interval '1 minute',attempt=attempt+1 WHERE (organization_id,delivery_id)=(
SELECT organization_id,delivery_id FROM github_events WHERE organization_id=$1 AND processed_at IS NULL AND attempt<10 AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY received_at FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING delivery_id,event,payload,attempt`, repository.Organization, event.Lease).Scan(&event.ID, &event.Name, &event.Payload, &event.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &event, err
}

// FinishEvent fences stale workers and records only generic errors, never untrusted payload text.
func (repository *Repository) FinishEvent(ctx context.Context, event Event, success bool, retry time.Duration) error {
	var result pgxResult
	var err error
	if success {
		result, err = repository.pool.Exec(ctx, `UPDATE github_events SET processed_at=now(),lease_token='',lease_until=NULL,error='' WHERE organization_id=$1 AND delivery_id=$2 AND lease_token=$3`, repository.Organization, event.ID, event.Lease)
	} else {
		result, err = repository.pool.Exec(ctx, `UPDATE github_events SET available_at=now()+$4::interval,lease_token='',lease_until=NULL,error='Webhook processing unavailable; retry required' WHERE organization_id=$1 AND delivery_id=$2 AND lease_token=$3`, repository.Organization, event.ID, event.Lease, retry.String())
	}
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return access.ErrConflict
	}
	return nil
}

type pgxResult interface{ RowsAffected() int64 }

// EventCounts exposes queue health without returning raw webhook bodies.
func (repository *Repository) EventCounts(ctx context.Context) (pending, failed int, err error) {
	err = repository.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE processed_at IS NULL AND attempt<10),count(*) FILTER (WHERE processed_at IS NULL AND attempt>=10) FROM github_events WHERE organization_id=$1`, repository.Organization).Scan(&pending, &failed)
	return
}

// PruneEvents bounds successful raw-payload retention without touching unprocessed work.
func (repository *Repository) PruneEvents(ctx context.Context) error {
	_, err := repository.pool.Exec(ctx, `DELETE FROM github_events WHERE (organization_id,delivery_id) IN (SELECT organization_id,delivery_id FROM github_events WHERE organization_id=$1 AND processed_at<now()-interval '7 days' ORDER BY processed_at LIMIT 500)`, repository.Organization)
	return err
}
