package store

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Observe accepts evidence only for an exact current project allocation and generation.
func (s *Store) Observe(ctx context.Context, organization, cluster string, e database.Evidence) error {
	p, ok := s.Policies[e.ProjectID]
	if !ok || e.Scope != p.Scope || e.OrganizationID != organization || p.ClusterID != cluster || !slices.Contains([]string{"pending", "ready", "failed", "retained"}, e.State) || len(e.Reason) > 160 || len(e.UID) > 80 || e.ObservedAt.After(time.Now().Add(time.Minute)) || time.Since(e.ObservedAt) > time.Minute {
		return postgres.ErrDenied
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	result, err := s.Pool.Exec(ctx, `UPDATE database_allocations SET observation=$5,updated_at=now() WHERE organization_id=$1 AND project_id=$2 AND id=$3 AND generation=$4`, organization, e.ProjectID, e.ID, e.Generation, data)
	if err == nil && result.RowsAffected() != 1 {
		return postgres.ErrStale
	}
	return err
}
