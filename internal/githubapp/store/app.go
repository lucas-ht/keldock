package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
)

// AppState exposes public registration and synchronization metadata only.
type AppState struct {
	AppID       int64             `json:"appId"`
	ClientID    string            `json:"clientId"`
	Slug        string            `json:"slug"`
	Enabled     bool              `json:"enabled"`
	Version     int64             `json:"version"`
	Permissions map[string]string `json:"permissions"`
	SyncedAt    *time.Time        `json:"syncedAt"`
	SyncError   string            `json:"syncError"`
}

// State returns an empty disabled registration before initial configuration.
func (repository *Repository) State(ctx context.Context) (AppState, error) {
	state := AppState{Permissions: map[string]string{}}
	err := repository.pool.QueryRow(ctx, `SELECT app_id,client_id,slug,enabled,version,permissions,synced_at,sync_error FROM github_apps WHERE organization_id=$1`, repository.Organization).Scan(&state.AppID, &state.ClientID, &state.Slug, &state.Enabled, &state.Version, &state.Permissions, &state.SyncedAt, &state.SyncError)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	return state, err
}

// Seed establishes an operator-supplied private credential identity without remote grants.
// A disabled registration remains disabled across process restarts.
func (repository *Repository) Seed(ctx context.Context, app client.App, fingerprint string) error {
	result, err := repository.pool.Exec(ctx, `INSERT INTO github_apps (organization_id,app_id,client_id,slug,fingerprint) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (organization_id) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,version=github_apps.version+CASE WHEN github_apps.fingerprint<>EXCLUDED.fingerprint THEN 1 ELSE 0 END
WHERE github_apps.app_id=EXCLUDED.app_id AND github_apps.enabled`, repository.Organization, app.ID, app.ClientID, app.Slug, fingerprint)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return access.ErrDenied
	}
	return nil
}

// Configure atomically audits current administrator authority with the credential write.
func (repository *Repository) Configure(ctx context.Context, principal access.Principal, expected int64, app client.App, fingerprint string, persist func() error) error {
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
	permissions, err := json.Marshal(app.Permissions)
	if err != nil {
		return err
	}
	if err := persist(); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO github_apps (organization_id,app_id,client_id,slug,fingerprint,permissions) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (organization_id) DO UPDATE SET app_id=EXCLUDED.app_id,client_id=EXCLUDED.client_id,slug=EXCLUDED.slug,fingerprint=EXCLUDED.fingerprint,permissions=EXCLUDED.permissions,enabled=true,version=github_apps.version+1,synced_at=NULL,sync_error=''`, repository.Organization, app.ID, app.ClientID, app.Slug, fingerprint, permissions); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE github_installations SET active=false WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE github_repositories SET active=false WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	if err := access.RecordAudit(ctx, tx, repository.Organization, "user:"+user.ID, "github.app_configured", app.Slug); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Disable revokes local delegation; GitHub's App and running workloads are retained.
func (repository *Repository) Disable(ctx context.Context, principal access.Principal, expected int64, remove func() error) error {
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
	if _, err := tx.Exec(ctx, `UPDATE github_apps SET enabled=false,version=version+1 WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE github_repositories SET active=false WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	if err := remove(); err != nil {
		return err
	}
	if err := access.RecordAudit(ctx, tx, repository.Organization, "user:"+user.ID, "github.app_disconnected", "installation"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SyncFailure records a bounded, credential-free recovery hint without changing grants.
func (repository *Repository) SyncFailure(ctx context.Context) error {
	_, err := repository.pool.Exec(ctx, `UPDATE github_apps SET sync_error='GitHub synchronization unavailable; retry after checking connection and App access' WHERE organization_id=$1`, repository.Organization)
	return err
}
