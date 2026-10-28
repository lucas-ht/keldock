package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
)

// ReviewDatabaseRecovery preserves retained allocations missing/newer than a snapshot.
func (s *Store) ReviewDatabaseRecovery(ctx context.Context, organization string, entries []v1.RetainedDatabaseCheckpoint) error {
	for _, entry := range entries {
		if entry.OrganizationID != organization {
			continue
		}
		var generation int64
		var data, intent []byte
		if err := s.pool.QueryRow(ctx, `SELECT generation,observation,intent FROM database_allocations WHERE organization_id=$1 AND project_id=$2 AND id=$3`, organization, entry.ProjectID, entry.ID).Scan(&generation, &data, &intent); err != nil {
			return fmt.Errorf("retained allocation %s is absent from restored state", entry.ID)
		}
		if entry.Generation > generation {
			return fmt.Errorf("retained allocation %s is newer than restored state", entry.ID)
		}
		var allocation database.Allocation
		if json.Unmarshal(intent, &allocation) != nil {
			return fmt.Errorf("invalid saved allocation")
		}
		if allocation.SourceUID != "" && allocation.SourceUID != entry.UID {
			return fmt.Errorf("retained allocation %s changed or lost its source identity", entry.ID)
		}
		if len(data) > 0 {
			var evidence database.Evidence
			if json.Unmarshal(data, &evidence) != nil {
				return fmt.Errorf("invalid saved allocation evidence")
			}
			if evidence.UID != "" && entry.UID != "" && evidence.UID != entry.UID {
				return fmt.Errorf("retained allocation %s changed identity", entry.ID)
			}
		}
	}
	return nil
}

// RecoveryInventory produces nonsecret, deterministic inputs for an operator's review hash.
func (s *Store) RecoveryInventory(ctx context.Context, organization string) (json.RawMessage, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT jsonb_build_object('accounts',coalesce((SELECT jsonb_agg(jsonb_build_object('id',u.id,'email',u.email,'role',u.role,'status',u.status,'version',u.version,'projects',coalesce((SELECT jsonb_agg(g.project_id ORDER BY g.project_id) FROM access_user_projects g WHERE g.user_id=u.id),'[]'::jsonb)) ORDER BY u.id) FROM access_users u WHERE u.organization_id=$1),'[]'::jsonb),'environments',coalesce((SELECT jsonb_agg(jsonb_build_object('id',e.id,'project',e.project_id,'generation',e.generation,'deleted',e.deleted,'state',o.state,'bundle',d.bundle) ORDER BY e.id) FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation LEFT JOIN deployments d ON d.id=o.deployment_id WHERE e.organization_id=$1),'[]'::jsonb),'databases',coalesce((SELECT jsonb_agg(jsonb_build_object('project',project_id,'id',id,'generation',generation,'action',intent->>'action') ORDER BY project_id,id) FROM database_allocations WHERE organization_id=$1),'[]'::jsonb),'cloud',coalesce((SELECT jsonb_agg(jsonb_build_object('project',project_id,'id',id,'generation',generation,'action',intent->>'action') ORDER BY project_id,id) FROM cloud_intents WHERE organization_id=$1),'[]'::jsonb))`, organization).Scan(&data)
	return data, err
}
