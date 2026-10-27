// Package mcpidentity owns the bounded OAuth authorization profile for MCP read access.
package mcpidentity

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucas-ht/keldock/internal/access"
)

// ReadScope is the only permission exposed by the first MCP adapter.
const ReadScope = "keldock:read"

// Repository binds all OAuth credentials to one organization and canonical MCP resource.
type Repository struct {
	pool     *pgxpool.Pool
	users    *access.Repository
	Issuer   string
	Resource string
}

// NewRepository permits HTTPS or explicit loopback HTTP installation origins.
func NewRepository(pool *pgxpool.Pool, users *access.Repository, origin string) (*Repository, error) {
	parsed, err := url.Parse(origin)
	if pool == nil || users == nil || err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, access.ErrInvalid
	}
	local := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !local) {
		return nil, access.ErrInvalid
	}
	issuer := strings.TrimSuffix(origin, "/")
	return &Repository{pool: pool, users: users, Issuer: issuer, Resource: issuer + "/mcp"}, nil
}

// Seed establishes an initially disabled MCP endpoint, without creating a client or grant.
func (repository *Repository) Seed(ctx context.Context) error {
	_, err := repository.pool.Exec(ctx, `INSERT INTO access_mcp_settings (organization_id) VALUES ($1) ON CONFLICT DO NOTHING`, repository.users.Organization)
	return err
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("MCP authorization rollback", "error", err)
	}
}

func (repository *Repository) requireEnabled(ctx context.Context, tx pgx.Tx) error {
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT enabled FROM access_mcp_settings WHERE organization_id=$1 FOR SHARE`, repository.users.Organization).Scan(&enabled)
	if err != nil {
		return err
	}
	if !enabled {
		return access.ErrDenied
	}
	return nil
}

func (repository *Repository) requireHuman(ctx context.Context, tx pgx.Tx, principal access.Principal) (access.User, error) {
	return repository.users.RequireHuman(ctx, tx, principal)
}

func clientActive(ctx context.Context, tx pgx.Tx, organization, id string) error {
	var exists string
	if err := tx.QueryRow(ctx, `SELECT id FROM access_mcp_clients WHERE organization_id=$1 AND id=$2 AND disabled_at IS NULL FOR SHARE`, organization, id).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
		return access.ErrDenied
	} else {
		return err
	}
}
