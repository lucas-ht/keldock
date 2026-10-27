// Package postgres stores admitted state and dispatch in the same PostgreSQL transaction.
package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Expected state errors are safe to classify at the API boundary.
var (
	ErrNotFound = errors.New("target not found")
	ErrConflict = errors.New("conflicting request")
	ErrStale    = errors.New("stale desired revision")
	ErrDenied   = errors.New("request outside authorized scope")
)

// Store owns a bounded connection pool; callers must close it during shutdown.
type Store struct{ pool *pgxpool.Pool }

// Pool shares the installation's bounded pool with its scoped persistence modules.
// The owning Store remains responsible for closing it.
func (store *Store) Pool() *pgxpool.Pool { return store.pool }

// Open connects without changing the schema or creating installation ownership.
func Open(ctx context.Context, connection string) (*Store, error) {
	config, err := pgxpool.ParseConfig(connection)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database unavailable: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (store *Store) Close() { store.pool.Close() }

// Migrate applies embedded, checksummed migrations under a transaction-level lock.
func (store *Store) Migrate(ctx context.Context) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(52718481); CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, checksum text NOT NULL)`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		data, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		var existing string
		err = tx.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, entry.Name()).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("migration checksum changed: %s", entry.Name())
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations VALUES ($1,$2)`, entry.Name(), checksum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("transaction rollback", "error", err)
	}
}

func newID(prefix string) string { return prefix + "-" + strings.ToLower(rand.Text()) }

// SeedProject explicitly establishes operator-owned organization/repository placement.
// Existing ownership cannot be silently reassigned by restarting the process.
func (store *Store) SeedProject(ctx context.Context, project v1.Project) error {
	if !v1.ValidName(project.OrganizationID) || !v1.ValidName(project.ProjectID) || !v1.ValidName(project.ClusterID) || project.RepositoryID <= 0 {
		return ErrDenied
	}
	_, err := store.pool.Exec(ctx, `INSERT INTO projects (organization_id,id,name,repository_id,cluster_id) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, project.OrganizationID, project.ProjectID, project.Name, project.RepositoryID, project.ClusterID)
	if err != nil {
		return err
	}
	var repository int64
	var cluster string
	err = store.pool.QueryRow(ctx, `SELECT repository_id,cluster_id FROM projects WHERE organization_id=$1 AND id=$2`, project.OrganizationID, project.ProjectID).Scan(&repository, &cluster)
	if err != nil || repository != project.RepositoryID || cluster != project.ClusterID {
		return ErrConflict
	}
	return nil
}
