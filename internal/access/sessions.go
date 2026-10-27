package access

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SessionLifetime is the absolute browser-session ceiling, independent of its idle timeout.
const SessionLifetime = 12 * time.Hour

func insertSession(ctx context.Context, tx pgx.Tx, user User) (string, error) {
	token := NewToken("kds_")
	_, err := tx.Exec(ctx, `INSERT INTO access_sessions (token_hash,user_id,user_version,expires_at) VALUES ($1,$2,$3,now()+interval '12 hours')`, TokenHash(token), user.ID, user.Version)
	return token, err
}

// LoginSession checks the password-verified account revision atomically with session creation.
func (repository *Repository) LoginSession(ctx context.Context, verified User) (string, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)
	var current int64
	if err := tx.QueryRow(ctx, `SELECT version FROM access_users WHERE organization_id=$1 AND id=$2 AND status='active' FOR UPDATE`, repository.Organization, verified.ID).Scan(&current); err != nil || current != verified.Version {
		return "", ErrDenied
	}
	if _, err := tx.Exec(ctx, `UPDATE access_users SET last_login_at=now() WHERE id=$1`, verified.ID); err != nil {
		return "", err
	}
	token, err := insertSession(ctx, tx, verified)
	if err != nil {
		return "", err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "user:"+verified.ID, "session.created", verified.ID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return token, repository.ClearLoginLimit(ctx, verified.Email)
}

// Session validates expiry, revocation and the live account revision on every request.
func (repository *Repository) Session(ctx context.Context, token string) (Principal, time.Time, error) {
	if !strings.HasPrefix(token, "kds_") || len(token) != 47 {
		return Principal{}, time.Time{}, ErrDenied
	}
	var id string
	var version int64
	var expires time.Time
	err := repository.pool.QueryRow(ctx, `UPDATE access_sessions s SET last_used_at=now() FROM access_users u WHERE s.token_hash=$1 AND u.id=s.user_id AND u.organization_id=$2 AND u.status='active' AND s.user_version=u.version AND s.revoked_at IS NULL AND s.expires_at>now() AND s.last_used_at>now()-interval '30 minutes' RETURNING s.user_id,s.user_version,s.expires_at`, TokenHash(token), repository.Organization).Scan(&id, &version, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, time.Time{}, ErrDenied
	}
	if err != nil {
		return Principal{}, time.Time{}, err
	}
	user, err := repository.User(ctx, id)
	if err != nil || user.Status != "active" || user.Version != version {
		return Principal{}, time.Time{}, ErrDenied
	}
	return Principal{User: user, Kind: "session"}, expires, nil
}

// Logout revokes the current browser session without changing other credentials.
func (repository *Repository) Logout(ctx context.Context, token string) error {
	_, err := repository.pool.Exec(ctx, `UPDATE access_sessions s SET revoked_at=now() FROM access_users u WHERE s.token_hash=$1 AND u.id=s.user_id AND u.organization_id=$2`, TokenHash(token), repository.Organization)
	return err
}

// AllowLoginAttempt applies a durable account-scoped limit shared across API processes.
func (repository *Repository) AllowLoginAttempt(ctx context.Context, email string) error {
	var attempts int
	err := repository.pool.QueryRow(ctx, `INSERT INTO access_login_limits (organization_id,bucket,attempts) VALUES ($1,$2,1)
ON CONFLICT (organization_id,bucket) DO UPDATE SET attempts=CASE WHEN access_login_limits.window_start<now()-interval '15 minutes' THEN 1 ELSE access_login_limits.attempts+1 END,
window_start=CASE WHEN access_login_limits.window_start<now()-interval '15 minutes' THEN now() ELSE access_login_limits.window_start END RETURNING attempts`, repository.Organization, TokenHash(email)).Scan(&attempts)
	if err != nil {
		return err
	}
	if attempts > 6 {
		return ErrLimited
	}
	return nil
}

// ClearLoginLimit clears only the successfully authenticated account's failure window.
func (repository *Repository) ClearLoginLimit(ctx context.Context, email string) error {
	_, err := repository.pool.Exec(ctx, `DELETE FROM access_login_limits WHERE organization_id=$1 AND bucket=$2`, repository.Organization, TokenHash(email))
	return err
}
