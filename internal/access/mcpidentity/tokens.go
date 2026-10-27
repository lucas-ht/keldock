package mcpidentity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

func validVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	return strings.IndexFunc(value, func(char rune) bool {
		allowed := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("-._~", char)
		return !allowed
	}) < 0
}

func (repository *Repository) grant(ctx context.Context, tx pgx.Tx, id, client string) (access.Principal, time.Time, error) {
	var userID, resource string
	var version int64
	var projects []byte
	var created time.Time
	err := tx.QueryRow(ctx, `SELECT user_id,user_version,project_ids,resource,created_at FROM access_mcp_grants WHERE id=$1 AND client_id=$2 AND revoked_at IS NULL AND created_at>now()-interval '30 days'`, id, client).Scan(&userID, &version, &projects, &resource, &created)
	if err != nil || resource != repository.Resource {
		return access.Principal{}, created, access.ErrDenied
	}
	var current int64
	var status string
	err = tx.QueryRow(ctx, `SELECT version,status FROM access_users WHERE id=$1 AND organization_id=$2 FOR SHARE`, userID, repository.users.Organization).Scan(&current, &status)
	if err != nil || current != version || status != "active" {
		return access.Principal{}, created, access.ErrDenied
	}
	// Lock the user before the grant, matching account-revocation lock ordering.
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM access_mcp_grants WHERE id=$1 AND client_id=$2 AND revoked_at IS NULL FOR UPDATE`, id, client).Scan(&locked); err != nil {
		return access.Principal{}, created, access.ErrDenied
	}
	user, err := access.LoadUser(ctx, tx, repository.users.Organization, userID)
	if err != nil {
		return access.Principal{}, created, err
	}
	principal := access.Principal{User: user, Kind: "mcp", GrantID: id}
	if err := json.Unmarshal(projects, &principal.TokenProjects); err != nil {
		return principal, created, err
	}
	return principal, created, nil
}

func issueTokens(ctx context.Context, tx pgx.Tx, grant string, created time.Time) (TokenPair, error) {
	remaining := time.Until(created.Add(30 * 24 * time.Hour))
	if remaining <= 0 {
		return TokenPair{}, access.ErrDenied
	}
	lifetime := min(time.Hour, remaining)
	pair := TokenPair{AccessToken: access.NewToken("kma_"), TokenType: "Bearer", ExpiresIn: int64(lifetime.Seconds()), RefreshToken: access.NewToken("kmr_"), Scope: ReadScope}
	_, err := tx.Exec(ctx, `INSERT INTO access_mcp_tokens (token_hash,grant_id,kind,expires_at) VALUES ($1,$2,'access',now()+make_interval(secs=>$3)),($4,$2,'refresh',$5)`, access.TokenHash(pair.AccessToken), grant, lifetime.Seconds(), access.TokenHash(pair.RefreshToken), created.Add(30*24*time.Hour))
	return pair, err
}

// Exchange enforces one-time codes, exact redirect/resource binding and S256 proof.
func (repository *Repository) Exchange(ctx context.Context, client, redirect, resource, code, verifier string) (TokenPair, error) {
	if resource != repository.Resource || !validVerifier(verifier) || len(code) > 128 {
		return TokenPair{}, access.ErrInvalid
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	defer rollback(ctx, tx)
	if err := repository.requireEnabled(ctx, tx); err != nil {
		return TokenPair{}, err
	}
	if err := clientActive(ctx, tx, repository.users.Organization, client); err != nil {
		return TokenPair{}, err
	}
	var grant, callback, challenge string
	var consumed *time.Time
	err = tx.QueryRow(ctx, `SELECT o.grant_id,o.redirect_uri,o.challenge,o.consumed_at FROM access_oauth_codes o JOIN access_mcp_grants g ON g.id=o.grant_id WHERE o.token_hash=$1 AND g.client_id=$2 AND o.expires_at>now() FOR UPDATE OF o`, access.TokenHash(code), client).Scan(&grant, &callback, &challenge, &consumed)
	if err != nil {
		return TokenPair{}, access.ErrDenied
	}
	digest := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(digest[:])
	if callback != redirect || subtle.ConstantTimeCompare([]byte(challenge), []byte(expected)) != 1 {
		return TokenPair{}, access.ErrDenied
	}
	if consumed != nil {
		if err := revokeReplay(ctx, tx, repository.users.Organization, grant, "mcp.code_replayed"); err != nil {
			return TokenPair{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return TokenPair{}, err
		}
		return TokenPair{}, access.ErrDenied
	}
	_, created, err := repository.grant(ctx, tx, grant, client)
	if err != nil {
		return TokenPair{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_oauth_codes SET consumed_at=now() WHERE token_hash=$1`, access.TokenHash(code)); err != nil {
		return TokenPair{}, err
	}
	pair, err := issueTokens(ctx, tx, grant, created)
	if err != nil {
		return TokenPair{}, err
	}
	return pair, tx.Commit(ctx)
}

func revokeReplay(ctx context.Context, tx pgx.Tx, organization, grant, event string) error {
	if _, err := tx.Exec(ctx, `UPDATE access_mcp_grants SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1`, grant); err != nil {
		return err
	}
	return access.RecordAudit(ctx, tx, organization, "oauth-client", event, grant)
}

// Refresh rotates refresh credentials; reuse revokes the whole connection family.
func (repository *Repository) Refresh(ctx context.Context, client, resource, refresh string) (TokenPair, error) {
	// RFC 8707 permits an omitted resource on refresh. The stored grant remains
	// bound to this exact installation; an explicit different resource is denied.
	if resource == "" {
		resource = repository.Resource
	}
	if resource != repository.Resource || len(refresh) > 128 {
		return TokenPair{}, access.ErrInvalid
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	defer rollback(ctx, tx)
	if err := repository.requireEnabled(ctx, tx); err != nil {
		return TokenPair{}, err
	}
	if err := clientActive(ctx, tx, repository.users.Organization, client); err != nil {
		return TokenPair{}, err
	}
	var grant string
	var consumed *time.Time
	err = tx.QueryRow(ctx, `SELECT t.grant_id,t.consumed_at FROM access_mcp_tokens t JOIN access_mcp_grants g ON g.id=t.grant_id WHERE t.token_hash=$1 AND t.kind='refresh' AND t.expires_at>now() AND g.client_id=$2 FOR UPDATE OF t`, access.TokenHash(refresh), client).Scan(&grant, &consumed)
	if err != nil {
		return TokenPair{}, access.ErrDenied
	}
	if consumed != nil {
		if err := revokeReplay(ctx, tx, repository.users.Organization, grant, "mcp.refresh_replayed"); err != nil {
			return TokenPair{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return TokenPair{}, err
		}
		return TokenPair{}, access.ErrDenied
	}
	_, created, err := repository.grant(ctx, tx, grant, client)
	if err != nil {
		return TokenPair{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_mcp_tokens SET consumed_at=now() WHERE token_hash=$1`, access.TokenHash(refresh)); err != nil {
		return TokenPair{}, err
	}
	pair, err := issueTokens(ctx, tx, grant, created)
	if err != nil {
		return TokenPair{}, err
	}
	return pair, tx.Commit(ctx)
}

// Authenticate validates audience, endpoint policy, revocation and current human authority.
func (repository *Repository) Authenticate(ctx context.Context, token, resource string) (access.Principal, time.Time, error) {
	if resource != repository.Resource || !strings.HasPrefix(token, "kma_") || len(token) != 47 {
		return access.Principal{}, time.Time{}, access.ErrDenied
	}
	var grant, userID string
	var version int64
	var projects []byte
	var expires time.Time
	err := repository.pool.QueryRow(ctx, `UPDATE access_mcp_grants g SET last_used_at=now() FROM access_mcp_tokens t,access_mcp_clients c,access_users u,access_mcp_settings s
WHERE t.token_hash=$1 AND t.kind='access' AND t.expires_at>now() AND t.grant_id=g.id AND g.client_id=c.id AND c.organization_id=$2 AND c.disabled_at IS NULL
AND g.revoked_at IS NULL AND g.resource=$3 AND g.created_at>now()-interval '30 days' AND u.id=g.user_id AND u.status='active' AND u.organization_id=$2 AND u.version=g.user_version
AND s.organization_id=$2 AND s.enabled RETURNING g.id,g.user_id,g.user_version,g.project_ids,t.expires_at`, access.TokenHash(token), repository.users.Organization, resource).Scan(&grant, &userID, &version, &projects, &expires)
	if err != nil {
		return access.Principal{}, expires, access.ErrDenied
	}
	user, err := repository.users.User(ctx, userID)
	if err != nil || user.Version != version || user.Status != "active" {
		return access.Principal{}, expires, access.ErrDenied
	}
	principal := access.Principal{User: user, Kind: "mcp", GrantID: grant}
	if err := json.Unmarshal(projects, &principal.TokenProjects); err != nil {
		return principal, expires, err
	}
	return principal, expires, nil
}
