package postgres

import (
	"context"
	"fmt"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// RecoveryFenced blocks all server startup after a supported database restoration.
func (s *Store) RecoveryFenced(ctx context.Context, organization string) (bool, error) {
	var fenced bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM recovery_state WHERE organization_id=$1 AND fenced)`, organization).Scan(&fenced)
	return fenced, err
}

// FenceRecovery is called on an independent restored database, never the running source.
func (s *Store) FenceRecovery(ctx context.Context, organization, snapshot string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if _, err := tx.Exec(ctx, `INSERT INTO recovery_state(organization_id,snapshot_id) VALUES($1,$2) ON CONFLICT(organization_id) DO UPDATE SET fenced=true,snapshot_id=EXCLUDED.snapshot_id,restored_at=now(),resumed_at=NULL`, organization, snapshot); err != nil {
		return err
	}
	// Recovered bearer/browser credentials and pending enrollments cannot regain authority.
	for _, table := range []string{"access_agents", "access_cloud_agents", "access_database_agents"} {
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET revoked_at=now(),session_until=NULL WHERE organization_id=$1`, organization); err != nil {
			return err
		}
	}
	for _, table := range []string{"access_enrollments", "access_cloud_enrollments", "access_database_enrollments"} {
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET consumed_at=now() WHERE organization_id=$1`, organization); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE access_sessions SET revoked_at=now() WHERE user_id IN(SELECT id FROM access_users WHERE organization_id=$1)`, organization); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_mcp_grants SET revoked_at=now() WHERE user_id IN(SELECT id FROM access_users WHERE organization_id=$1)`, organization); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_invitations SET consumed_at=now() WHERE user_id IN(SELECT id FROM access_users WHERE organization_id=$1)`, organization); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE operations o SET lease_token='',lease_until=NULL WHERE EXISTS(SELECT 1 FROM environments e WHERE e.id=o.environment_id AND e.organization_id=$1)`, organization); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReviewRecovery rejects stale generations, deleted-preview resurrection and changed artifact identity.
// Missing checkpoints are accepted only for work that never reached a cluster effect.
func (s *Store) ReviewRecovery(ctx context.Context, organization string, entries []v1.RecoveryCheckpoint) error {
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.OrganizationID != "" && entry.OrganizationID != organization {
			continue
		}
		if seen[entry.EnvironmentID] {
			return fmt.Errorf("duplicate cluster recovery checkpoint")
		}
		seen[entry.EnvironmentID] = true
		var generation int64
		var deleted bool
		var deployment, bundle, state string
		err := s.pool.QueryRow(ctx, `SELECT e.generation,e.deleted,coalesce(d.id,''),coalesce(d.bundle,''),o.state FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation LEFT JOIN deployments d ON d.id=o.deployment_id WHERE e.organization_id=$1 AND e.id=$2`, organization, entry.EnvironmentID).Scan(&generation, &deleted, &deployment, &bundle, &state)
		if err != nil {
			return fmt.Errorf("surviving environment %s is absent from restored state", entry.EnvironmentID)
		}
		if generation < entry.Generation || (entry.Action == "delete" && !deleted) || (entry.Action == "suspend" && generation == entry.Generation && state != "canceled") || (generation == entry.Generation && entry.Action == "deploy" && (deployment != entry.DeploymentID || bundle != entry.Bundle)) {
			return fmt.Errorf("restored state conflicts with surviving environment %s; retain the fence and restore a newer snapshot", entry.EnvironmentID)
		}
	}
	return nil
}

// ResumeRecovery requires an externally verified cluster checkpoint review before startup.
func (s *Store) ResumeRecovery(ctx context.Context, organization string, entries []v1.RecoveryCheckpoint) error {
	if err := s.ReviewRecovery(ctx, organization, entries); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE recovery_state SET fenced=false,resumed_at=now() WHERE organization_id=$1 AND fenced`, organization)
	return err
}
