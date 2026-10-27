package mcpidentity

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

// Connections exposes a bounded page of grant metadata, never access or refresh secrets.
func (repository *Repository) Connections(ctx context.Context, principal access.Principal, all bool, after string) ([]Connection, string, error) {
	if principal.Kind != "session" || principal.User.OrganizationID != repository.users.Organization || (all && !principal.IsAdmin()) {
		return nil, "", access.ErrDenied
	}
	rows, err := repository.pool.Query(ctx, `SELECT g.id,c.id,c.name,u.id,u.name,g.project_ids,g.resource,g.created_at,g.created_at+interval '30 days',g.last_used_at,g.revoked_at
FROM access_mcp_grants g JOIN access_mcp_clients c ON c.id=g.client_id JOIN access_users u ON u.id=g.user_id
WHERE c.organization_id=$1 AND (g.user_id=$2 OR $3) AND g.id>$4 ORDER BY g.id LIMIT 51`, repository.users.Organization, principal.User.ID, all, after)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	connections := []Connection{}
	for rows.Next() {
		var connection Connection
		var projects []byte
		if err := rows.Scan(&connection.ID, &connection.ClientID, &connection.ClientName, &connection.UserID, &connection.UserName, &projects, &connection.Resource, &connection.CreatedAt, &connection.ExpiresAt, &connection.LastUsedAt, &connection.RevokedAt); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(projects, &connection.Projects); err != nil {
			return nil, "", err
		}
		connections = append(connections, connection)
	}
	var next string
	if len(connections) > 50 {
		connections = connections[:50]
		next = connections[49].ID
	}
	return connections, next, rows.Err()
}

// RevokeConnection permits a human to revoke their own grant, or an admin to revoke any.
func (repository *Repository) RevokeConnection(ctx context.Context, principal access.Principal, id string) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	user, err := repository.requireHuman(ctx, tx, principal)
	if err != nil {
		return err
	}
	var owner string
	err = tx.QueryRow(ctx, `SELECT g.user_id FROM access_mcp_grants g JOIN access_mcp_clients c ON c.id=g.client_id WHERE g.id=$1 AND c.organization_id=$2 FOR UPDATE OF g`, id, repository.users.Organization).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner != user.ID && user.Role != "admin" {
		return access.ErrDenied
	}
	if _, err := tx.Exec(ctx, `UPDATE access_mcp_grants SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1`, id); err != nil {
		return err
	}
	if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+user.ID, "mcp.connection_revoked", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
