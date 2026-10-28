package mcpidentity

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

// OperationScope requires explicit consent to both inspection and bounded operations.
const OperationScope = ReadScope + " " + OperateScope

// NormalizeScope accepts the supported explicit permissions without expanding a read grant.
func NormalizeScope(input string) (string, error) {
	if input == "" {
		return ReadScope, nil
	}
	seen := map[string]bool{}
	for _, scope := range strings.Fields(input) {
		if seen[scope] || (scope != ReadScope && scope != OperateScope) {
			return "", access.ErrInvalid
		}
		seen[scope] = true
	}
	if !seen[ReadScope] {
		return "", access.ErrInvalid
	}
	if seen[OperateScope] {
		return OperationScope, nil
	}
	return ReadScope, nil
}

func matchesScope(granted string, requested []string) bool {
	if len(requested) == 0 || (len(requested) == 1 && requested[0] == "") {
		return true
	}
	if len(requested) != 1 {
		return false
	}
	scope, err := NormalizeScope(requested[0])
	return err == nil && scope == granted
}

// RequireOperator holds current account, grant, client and endpoint authority until commit.
func (repository *Repository) RequireOperator(ctx context.Context, tx pgx.Tx, principal access.Principal, project string) error {
	if principal.Kind != "mcp" {
		return access.ErrDenied
	}
	user, err := repository.users.CurrentUser(ctx, tx, principal)
	if err != nil {
		return err
	}
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT g.scope=$4 AND g.project_ids ? $5 FROM access_mcp_grants g JOIN access_mcp_clients c ON c.id=g.client_id JOIN access_mcp_settings s ON s.organization_id=c.organization_id
WHERE g.id=$1 AND g.user_id=$2 AND g.user_version=$3 AND g.revoked_at IS NULL AND g.resource=$6 AND g.created_at>now()-interval '30 days'
AND c.organization_id=$7 AND c.disabled_at IS NULL AND s.enabled FOR SHARE OF g,c,s`, principal.GrantID, user.ID, user.Version, OperationScope, project, repository.Resource, repository.users.Organization).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !allowed) {
		return access.ErrDenied
	}
	if err != nil {
		return err
	}
	current := access.Principal{User: user, Kind: "session"}
	if !current.CanOperate(project) {
		return access.ErrDenied
	}
	return nil
}
