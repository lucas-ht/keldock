package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

// BeginSetup binds an expiring nonce to this administrator and configuration version.
func (repository *Repository) BeginSetup(ctx context.Context, principal access.Principal, expected int64, purpose string) (string, error) {
	if purpose != "manifest" && purpose != "installation" {
		return "", access.ErrInvalid
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)
	user, err := repository.administrator(ctx, tx, principal)
	if err != nil {
		return "", err
	}
	if err := repository.version(ctx, tx, expected); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM github_setup_requests WHERE organization_id=$1 AND expires_at<now()`, repository.Organization); err != nil {
		return "", err
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM github_setup_requests WHERE organization_id=$1 AND consumed_at IS NULL`, repository.Organization).Scan(&pending); err != nil {
		return "", err
	}
	if pending >= 20 {
		return "", access.ErrLimited
	}
	state := rand.Text() + rand.Text()
	hash := sha256.Sum256([]byte(state))
	if _, err := tx.Exec(ctx, `INSERT INTO github_setup_requests (state_hash,organization_id,user_id,user_version,purpose,expires_at,app_version) VALUES ($1,$2,$3,$4,$5,now()+interval '10 minutes',$6)`, hash[:], repository.Organization, user.ID, user.Version, purpose, expected); err != nil {
		return "", err
	}
	return state, tx.Commit(ctx)
}

// ConsumeSetup refuses callback replay, stale admin sessions and changed App configuration.
func (repository *Repository) ConsumeSetup(ctx context.Context, principal access.Principal, state, purpose string) (int64, error) {
	if len(state) < 40 || len(state) > 128 {
		return 0, access.ErrDenied
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer rollback(ctx, tx)
	user, err := repository.administrator(ctx, tx, principal)
	if err != nil {
		return 0, err
	}
	hash := sha256.Sum256([]byte(state))
	var version, userVersion int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT app_version,user_version,expires_at FROM github_setup_requests WHERE state_hash=$1 AND organization_id=$2 AND user_id=$3 AND purpose=$4 AND consumed_at IS NULL FOR UPDATE`, hash[:], repository.Organization, user.ID, purpose).Scan(&version, &userVersion, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, access.ErrDenied
	}
	if err != nil {
		return 0, err
	}
	if userVersion != user.Version || time.Now().After(expires) {
		return 0, access.ErrDenied
	}
	if err := repository.version(ctx, tx, version); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE github_setup_requests SET consumed_at=now() WHERE state_hash=$1`, hash[:]); err != nil {
		return 0, err
	}
	return version, tx.Commit(ctx)
}

// AdminAction enforces current authority and records bounded administrative API effects.
func (repository *Repository) AdminAction(ctx context.Context, principal access.Principal, expected int64, event string, action func() error) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	user, err := repository.administrator(ctx, tx, principal)
	if err != nil {
		return err
	}
	if err := repository.version(ctx, tx, expected); err != nil {
		return err
	}
	if err := action(); err != nil {
		return err
	}
	if err := access.RecordAudit(ctx, tx, repository.Organization, "user:"+user.ID, event, "github"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
