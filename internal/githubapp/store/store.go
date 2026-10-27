// Package store persists GitHub delegation, incoming work and feedback independently of HTTP clients.
package store

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucas-ht/keldock/internal/access"
)

// Repository restricts every integration query to one installation organization.
type Repository struct {
	pool         *pgxpool.Pool
	users        *access.Repository
	Organization string
}

// New shares the product pool and account authority without owning their lifecycle.
func New(pool *pgxpool.Pool, users *access.Repository) (*Repository, error) {
	if pool == nil || users == nil {
		return nil, access.ErrInvalid
	}
	return &Repository{pool: pool, users: users, Organization: users.Organization}, nil
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("GitHub transaction rollback", "error", err)
	}
}

func (repository *Repository) administrator(ctx context.Context, tx pgx.Tx, principal access.Principal) (access.User, error) {
	user, err := repository.users.RequireHuman(ctx, tx, principal)
	if err != nil {
		return user, err
	}
	if user.Role != "admin" {
		return user, access.ErrDenied
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, repository.Organization+"/github/configuration"); err != nil {
		return user, err
	}
	return user, nil
}

func (repository *Repository) version(ctx context.Context, tx pgx.Tx, expected int64) error {
	var version int64
	err := tx.QueryRow(ctx, `SELECT version FROM github_apps WHERE organization_id=$1 FOR UPDATE`, repository.Organization).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) && expected == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	if version != expected {
		return access.ErrConflict
	}
	return nil
}
