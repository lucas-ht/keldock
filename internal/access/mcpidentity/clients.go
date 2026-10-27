package mcpidentity

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

// ValidRedirect accepts exact HTTPS callbacks and native loopback HTTP callbacks.
// No callback is fetched by Keldock, and fragments or embedded credentials are forbidden.
func ValidRedirect(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || strings.ContainsFunc(raw, unicode.IsControl) {
		return false
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	if parsed.Scheme == "https" {
		return true
	}
	loopback := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	return parsed.Scheme == "http" && loopback
}

// Register supports public PKCE clients; registration itself grants no user or project access.
func (repository *Repository) Register(ctx context.Context, name string, redirects []string) (Client, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) || len(redirects) < 1 || len(redirects) > 5 {
		return Client{}, access.ErrInvalid
	}
	seen := map[string]bool{}
	for _, redirect := range redirects {
		if !ValidRedirect(redirect) || seen[redirect] {
			return Client{}, access.ErrInvalid
		}
		seen[redirect] = true
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Client{}, err
	}
	defer rollback(ctx, tx)
	if err := repository.requireEnabled(ctx, tx); err != nil {
		return Client{}, err
	}
	data, err := json.Marshal(redirects)
	if err != nil {
		return Client{}, err
	}
	client := Client{ID: access.NewID("client"), Name: name, RedirectURIs: redirects}
	err = tx.QueryRow(ctx, `INSERT INTO access_mcp_clients (id,organization_id,name,redirect_uris) VALUES ($1,$2,$3,$4) RETURNING created_at`, client.ID, repository.users.Organization, name, data).Scan(&client.CreatedAt)
	if err != nil {
		return Client{}, err
	}
	return client, tx.Commit(ctx)
}

func readClient(ctx context.Context, query access.Querier, organization, id string) (Client, error) {
	var client Client
	var redirects []byte
	err := query.QueryRow(ctx, `SELECT id,name,redirect_uris,created_at,disabled_at FROM access_mcp_clients WHERE organization_id=$1 AND id=$2`, organization, id).Scan(&client.ID, &client.Name, &redirects, &client.CreatedAt, &client.DisabledAt)
	if err != nil {
		return client, err
	}
	err = json.Unmarshal(redirects, &client.RedirectURIs)
	return client, err
}

// Clients lists a bounded registration page for a human administrator.
func (repository *Repository) Clients(ctx context.Context, principal access.Principal, after string) ([]Client, string, error) {
	if !principal.IsAdmin() || principal.User.OrganizationID != repository.users.Organization {
		return nil, "", access.ErrDenied
	}
	rows, err := repository.pool.Query(ctx, `SELECT id,name,redirect_uris,created_at,disabled_at FROM access_mcp_clients WHERE organization_id=$1 AND id>$2 ORDER BY id LIMIT 51`, repository.users.Organization, after)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	clients := []Client{}
	for rows.Next() {
		var client Client
		var redirects []byte
		if err := rows.Scan(&client.ID, &client.Name, &redirects, &client.CreatedAt, &client.DisabledAt); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(redirects, &client.RedirectURIs); err != nil {
			return nil, "", err
		}
		clients = append(clients, client)
	}
	var next string
	if len(clients) > 50 {
		clients = clients[:50]
		next = clients[49].ID
	}
	return clients, next, rows.Err()
}

func approvedRedirect(client Client, redirect string) bool {
	return client.DisabledAt == nil && slices.Contains(client.RedirectURIs, redirect)
}

// DisableClient revokes all authorizations for that client and prevents further consent.
func (repository *Repository) DisableClient(ctx context.Context, principal access.Principal, id string) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	user, err := repository.requireHuman(ctx, tx, principal)
	if err != nil || user.Role != "admin" {
		return access.ErrDenied
	}
	var client string
	if err := tx.QueryRow(ctx, `UPDATE access_mcp_clients SET disabled_at=coalesce(disabled_at,now()) WHERE organization_id=$1 AND id=$2 RETURNING id`, repository.users.Organization, id).Scan(&client); err != nil {
		if err == pgx.ErrNoRows {
			return access.ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_mcp_grants SET revoked_at=coalesce(revoked_at,now()) WHERE client_id=$1`, client); err != nil {
		return err
	}
	if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+user.ID, "mcp.client_disabled", client); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
