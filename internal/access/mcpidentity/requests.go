package mcpidentity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// CreateAuthorization validates every redirect/resource/PKCE field before involving a browser.
func (repository *Repository) CreateAuthorization(ctx context.Context, input AuthorizationInput) (string, error) {
	challenge, err := base64.RawURLEncoding.DecodeString(input.Challenge)
	if err != nil || len(challenge) != 32 || len(input.Challenge) != 43 || input.ChallengeMethod != "S256" || input.ResponseType != "code" || input.Resource != repository.Resource || len(input.State) > 1024 || !utf8.ValidString(input.State) || strings.ContainsRune(input.State, 0) {
		return "", access.ErrInvalid
	}
	scope, err := NormalizeScope(input.Scope)
	if err != nil {
		return "", err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)
	if err := repository.requireEnabled(ctx, tx); err != nil {
		return "", err
	}
	client, err := readClient(ctx, tx, repository.users.Organization, input.ClientID)
	if err != nil || !approvedRedirect(client, input.RedirectURI) {
		return "", access.ErrDenied
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, repository.users.Organization+"/oauth/"+client.ID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM access_oauth_requests WHERE client_id=$1 AND expires_at<now()`, client.ID); err != nil {
		return "", err
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM access_oauth_requests WHERE client_id=$1 AND consumed_at IS NULL`, client.ID).Scan(&pending); err != nil {
		return "", err
	}
	if pending >= 20 {
		return "", access.ErrLimited
	}
	id := access.NewID("authreq")
	_, err = tx.Exec(ctx, `INSERT INTO access_oauth_requests (id,client_id,redirect_uri,state,challenge,resource,scope,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,now()+interval '5 minutes')`, id, client.ID, input.RedirectURI, input.State, input.Challenge, input.Resource, scope)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// Authorization exposes only the client/scope being approved, to a current human session.
func (repository *Repository) Authorization(ctx context.Context, principal access.Principal, id string) (AuthorizationRequest, error) {
	if principal.Kind != "session" || principal.User.OrganizationID != repository.users.Organization {
		return AuthorizationRequest{}, access.ErrDenied
	}
	var result AuthorizationRequest
	err := repository.pool.QueryRow(ctx, `SELECT r.id,c.id,c.name,r.redirect_uri,r.resource,r.scope,r.expires_at FROM access_oauth_requests r
JOIN access_mcp_clients c ON c.id=r.client_id JOIN access_mcp_settings s ON s.organization_id=c.organization_id
WHERE r.id=$1 AND c.organization_id=$2 AND c.disabled_at IS NULL AND s.enabled AND r.consumed_at IS NULL AND r.expires_at>now()`, id, repository.users.Organization).Scan(&result.ID, &result.ClientID, &result.ClientName, &result.RedirectURI, &result.Resource, &result.Scope, &result.ExpiresAt)
	if err == pgx.ErrNoRows {
		return result, access.ErrNotFound
	}
	return result, err
}

// Consent consumes an authorization request once and returns only its prevalidated callback.
// The one-time code is bound to client, exact callback, resource, PKCE and selected projects.
func (repository *Repository) Consent(ctx context.Context, principal access.Principal, id string, allow bool, projects []string) (string, error) {
	if len(projects) > 32 {
		return "", access.ErrInvalid
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)
	user, err := repository.requireHuman(ctx, tx, principal)
	if err != nil {
		return "", err
	}
	if err := repository.requireEnabled(ctx, tx); err != nil {
		return "", err
	}
	var client, redirect, state, challenge, resource, scope string
	err = tx.QueryRow(ctx, `SELECT r.client_id,r.redirect_uri,r.state,r.challenge,r.resource,r.scope FROM access_oauth_requests r JOIN access_mcp_clients c ON c.id=r.client_id
WHERE r.id=$1 AND c.organization_id=$2 AND c.disabled_at IS NULL AND r.consumed_at IS NULL AND r.expires_at>now() FOR UPDATE OF r FOR SHARE OF c`, id, repository.users.Organization).Scan(&client, &redirect, &state, &challenge, &resource, &scope)
	if err != nil {
		return "", access.ErrDenied
	}
	callback, err := url.Parse(redirect)
	if err != nil {
		return "", err
	}
	query := callback.Query()
	query.Set("state", state)
	query.Set("iss", repository.Issuer)
	if allow {
		if len(projects) == 0 {
			return "", access.ErrInvalid
		}
		current := access.Principal{User: user, Kind: "session"}
		seen := map[string]bool{}
		for _, project := range projects {
			if !v1.ValidName(project) || seen[project] || !current.CanRead(project) || (scope == OperationScope && !current.CanOperate(project)) {
				return "", access.ErrDenied
			}
			seen[project] = true
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE organization_id=$1 AND id=$2)`, repository.users.Organization, project).Scan(&exists); err != nil {
				return "", err
			}
			if !exists {
				return "", access.ErrDenied
			}
		}
		data, err := json.Marshal(projects)
		if err != nil {
			return "", err
		}
		grant, code := access.NewID("grant"), access.NewToken("kmc_")
		if _, err := tx.Exec(ctx, `INSERT INTO access_mcp_grants (id,client_id,user_id,user_version,project_ids,resource,scope) VALUES ($1,$2,$3,$4,$5,$6,$7)`, grant, client, user.ID, user.Version, data, resource, scope); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO access_oauth_codes (token_hash,grant_id,redirect_uri,challenge,expires_at) VALUES ($1,$2,$3,$4,now()+interval '60 seconds')`, access.TokenHash(code), grant, redirect, challenge); err != nil {
			return "", err
		}
		if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+user.ID, "mcp.authorized", grant); err != nil {
			return "", err
		}
		query.Set("code", code)
	} else {
		query.Set("error", "access_denied")
		if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+user.ID, "mcp.authorization_denied", client); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE access_oauth_requests SET consumed_at=now() WHERE id=$1`, id); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	callback.RawQuery = query.Encode()
	return callback.String(), nil
}
