// Package clusteridentity owns enrollment and revocable outbound-agent identities.
package clusteridentity

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Repository is bound to the installation's organization and single configured cluster.
type Repository struct {
	pool    *pgxpool.Pool
	users   *access.Repository
	cluster string
}

// NewRepository keeps cluster authority separate from browser/user credentials.
func NewRepository(pool *pgxpool.Pool, users *access.Repository, cluster string) (*Repository, error) {
	if pool == nil || users == nil || !v1.ValidName(cluster) {
		return nil, access.ErrInvalid
	}
	return &Repository{pool: pool, users: users, cluster: cluster}, nil
}

// Seed records only the operator-configured cluster, without adopting a running agent.
func (repository *Repository) Seed(ctx context.Context, name string) error {
	if name == "" || len(name) > 80 {
		return access.ErrInvalid
	}
	_, err := repository.pool.Exec(ctx, `INSERT INTO access_clusters (organization_id,id,name) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, repository.users.Organization, repository.cluster, name)
	return err
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("agent identity rollback", "error", err)
	}
}

func (repository *Repository) administrator(ctx context.Context, tx pgx.Tx, principal access.Principal) error {
	user, err := repository.users.RequireHuman(ctx, tx, principal)
	if err != nil {
		return err
	}
	if user.Role != "admin" {
		return access.ErrDenied
	}
	return nil
}

func (repository *Repository) lock(ctx context.Context, tx pgx.Tx) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `SELECT revision FROM access_clusters WHERE organization_id=$1 AND id=$2 FOR UPDATE`, repository.users.Organization, repository.cluster).Scan(&revision)
	return revision, err
}
