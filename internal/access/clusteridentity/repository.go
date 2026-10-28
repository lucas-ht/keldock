// Package clusteridentity owns enrollment and revocable outbound-agent identities.
package clusteridentity

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Repository is bound to the installation's organization and single configured cluster.
type Repository struct {
	pool     *pgxpool.Pool
	users    *access.Repository
	cluster  string
	cloud    bool
	database bool
}

// NewDatabaseRepository isolates database provisioning from workload and cloud authority.
func NewDatabaseRepository(pool *pgxpool.Pool, users *access.Repository, cluster string) (*Repository, error) {
	repository, err := NewRepository(pool, users, cluster)
	if err == nil {
		repository.database = true
	}
	return repository, err
}

// NewCloudRepository reuses the identity lifecycle with separate grants, credentials,
// revisions and leases. A workload credential cannot authenticate to this repository.
func NewCloudRepository(pool *pgxpool.Pool, users *access.Repository, cluster string) (*Repository, error) {
	repository, err := NewRepository(pool, users, cluster)
	if err == nil {
		repository.cloud = true
	}
	return repository, err
}

// statement selects only fixed, purpose-specific tables; no caller can choose SQL identifiers.
func (repository *Repository) statement(query string) string {
	if repository.database {
		return strings.NewReplacer("access_clusters", "access_database_clusters", "access_enrollments", "access_database_enrollments", "access_agents", "access_database_agents").Replace(query)
	}
	if !repository.cloud {
		return query
	}
	return strings.NewReplacer("access_clusters", "access_cloud_clusters", "access_enrollments", "access_cloud_enrollments", "access_agents", "access_cloud_agents").Replace(query)
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
	_, err := repository.pool.Exec(ctx, repository.statement(`INSERT INTO access_clusters (organization_id,id,name) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`), repository.users.Organization, repository.cluster, name)
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
	err := tx.QueryRow(ctx, repository.statement(`SELECT revision FROM access_clusters WHERE organization_id=$1 AND id=$2 FOR UPDATE`), repository.users.Organization, repository.cluster).Scan(&revision)
	return revision, err
}
