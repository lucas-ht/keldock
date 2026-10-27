package mcpidentity

import (
	"context"

	"github.com/lucas-ht/keldock/internal/access"
)

// Settings reads endpoint configuration for an already authenticated caller.
func (repository *Repository) Settings(ctx context.Context) (Settings, error) {
	settings := Settings{Endpoint: repository.Resource}
	err := repository.pool.QueryRow(ctx, `SELECT enabled,version FROM access_mcp_settings WHERE organization_id=$1`, repository.users.Organization).Scan(&settings.Enabled, &settings.Version)
	return settings, err
}

// SetEnabled rechecks administrator authority and revokes every grant when disabled.
func (repository *Repository) SetEnabled(ctx context.Context, principal access.Principal, enabled bool, version int64) (Settings, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Settings{}, err
	}
	defer rollback(ctx, tx)
	user, err := repository.requireHuman(ctx, tx, principal)
	if err != nil || user.Role != "admin" {
		return Settings{}, access.ErrDenied
	}
	settings := Settings{Enabled: enabled, Endpoint: repository.Resource}
	err = tx.QueryRow(ctx, `UPDATE access_mcp_settings SET enabled=$2,version=version+1,updated_at=now() WHERE organization_id=$1 AND version=$3 RETURNING version`, repository.users.Organization, enabled, version).Scan(&settings.Version)
	if err != nil {
		return Settings{}, access.ErrConflict
	}
	event := "mcp.enabled"
	if !enabled {
		event = "mcp.disabled"
		if _, err := tx.Exec(ctx, `UPDATE access_mcp_grants g SET revoked_at=now() FROM access_mcp_clients c WHERE g.client_id=c.id AND c.organization_id=$1 AND g.revoked_at IS NULL`, repository.users.Organization); err != nil {
			return Settings{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE access_oauth_requests r SET consumed_at=now() FROM access_mcp_clients c WHERE r.client_id=c.id AND c.organization_id=$1 AND r.consumed_at IS NULL`, repository.users.Organization); err != nil {
			return Settings{}, err
		}
	}
	if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+user.ID, event, repository.users.Organization); err != nil {
		return Settings{}, err
	}
	return settings, tx.Commit(ctx)
}
