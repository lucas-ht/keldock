// Package store persists project-scoped PostgreSQL allocations without database credentials.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Store has independently configured project policies; no absent capability is enabled.
type Store struct {
	Pool     *pgxpool.Pool
	Users    *access.Repository
	Policies map[string]database.Policy
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Debug("database transaction rollback", "error", err)
	}
}

// List returns retained data as well as active allocations within current project authority.
func (s *Store) List(ctx context.Context, p access.Principal, project string) ([]database.Allocation, error) {
	policy, ok := s.Policies[project]
	if !ok || policy.OrganizationID != p.User.OrganizationID || !p.CanRead(project) {
		return nil, access.ErrDenied
	}
	return s.list(ctx, policy.OrganizationID, policy.ClusterID, project)
}

// Desired is guarded by the separate database-agent authenticator.
func (s *Store) Desired(ctx context.Context, organization, cluster string) ([]database.Allocation, error) {
	return s.list(ctx, organization, cluster, "")
}

func (s *Store) list(ctx context.Context, organization, cluster, project string) ([]database.Allocation, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.intent,a.observation FROM database_allocations a JOIN projects p ON p.organization_id=a.organization_id AND p.id=a.project_id WHERE a.organization_id=$1 AND p.cluster_id=$2 AND ($3='' OR a.project_id=$3) ORDER BY a.project_id,a.id LIMIT 1001`, organization, cluster, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []database.Allocation{}
	for rows.Next() {
		var data, observation []byte
		var item database.Allocation
		if err := rows.Scan(&data, &observation); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if len(observation) > 0 {
			if err := json.Unmarshal(observation, &item.Observation); err != nil {
				return nil, err
			}
		}
		items = append(items, item)
	}
	if len(items) > 1000 {
		return nil, postgres.ErrQuota
	}
	return items, rows.Err()
}
