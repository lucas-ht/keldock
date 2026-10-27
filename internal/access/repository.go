package access

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/mail"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Repository restricts all identity queries to one operator-configured organization.
type Repository struct {
	pool         *pgxpool.Pool
	Organization string
}

// Querier allows identity modules to preserve one transaction across authorization and effects.
type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// NewRepository shares the installation's bounded connection pool without owning it.
func NewRepository(pool *pgxpool.Pool, organization string) (*Repository, error) {
	if pool == nil || !v1.ValidName(organization) {
		return nil, ErrInvalid
	}
	return &Repository{pool: pool, Organization: organization}, nil
}

// NewID generates a public bounded identifier, separate from credential verifiers.
func NewID(prefix string) string { return prefix + "-" + strings.ToLower(rand.Text()) }

const userColumns = `u.id,u.organization_id,u.email,u.name,u.role,u.status,u.version,u.created_at,u.last_login_at,
ARRAY(SELECT p.project_id FROM access_user_projects p WHERE p.user_id=u.id ORDER BY p.project_id)`

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.OrganizationID, &user.Email, &user.Name, &user.Role, &user.Status, &user.Version, &user.CreatedAt, &user.LastLoginAt, &user.Projects)
	return user, databaseError(err)
}

// LoadUser retrieves safe metadata within the caller's existing transaction when needed.
func LoadUser(ctx context.Context, query Querier, organization, id string) (User, error) {
	return scanUser(query.QueryRow(ctx, `SELECT `+userColumns+` FROM access_users u WHERE u.organization_id=$1 AND u.id=$2`, organization, id))
}

// User looks up account metadata without exposing password material.
func (repository *Repository) User(ctx context.Context, id string) (User, error) {
	return LoadUser(ctx, repository.pool, repository.Organization, id)
}

// RequireHuman revalidates account version and keeps its authority stable through a transaction.
func (repository *Repository) RequireHuman(ctx context.Context, tx pgx.Tx, principal Principal) (User, error) {
	if principal.Kind != "session" {
		return User{}, ErrDenied
	}
	return repository.CurrentUser(ctx, tx, principal)
}

// CurrentUser locks and revalidates current account authority for a shared transaction.
func (repository *Repository) CurrentUser(ctx context.Context, tx pgx.Tx, principal Principal) (User, error) {
	if principal.User.OrganizationID != repository.Organization {
		return User{}, ErrDenied
	}
	var version int64
	var status string
	err := tx.QueryRow(ctx, `SELECT version,status FROM access_users WHERE organization_id=$1 AND id=$2 FOR SHARE`, repository.Organization, principal.User.ID).Scan(&version, &status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}
	if err != nil || status != "active" || version != principal.User.Version {
		return User{}, ErrDenied
	}
	return LoadUser(ctx, tx, repository.Organization, principal.User.ID)
}

// RecordAudit writes credential-free identity history into the shared operation ledger.
func RecordAudit(ctx context.Context, tx pgx.Tx, organization, actor, event, target string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,project_id,actor,event,target,generation) VALUES ($1,'',$2,$3,$4,0)`, organization, actor, event, target)
	return err
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("identity transaction rollback", "error", err)
	}
}

func databaseError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) {
		switch postgres.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514":
			return ErrInvalid
		}
	}
	return err
}

func accountFields(email, name, role string, projects []string) (string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) || !slices.Contains([]string{"admin", "operator", "viewer"}, role) || len(projects) > 32 {
		return "", "", ErrInvalid
	}
	seen := map[string]bool{}
	for _, project := range projects {
		if !v1.ValidName(project) || seen[project] {
			return "", "", ErrInvalid
		}
		seen[project] = true
	}
	return email, name, nil
}

func replaceProjects(ctx context.Context, tx pgx.Tx, organization, user string, projects []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM access_user_projects WHERE user_id=$1`, user); err != nil {
		return err
	}
	for _, project := range projects {
		if _, err := tx.Exec(ctx, `INSERT INTO access_user_projects VALUES ($1,$2,$3)`, user, organization, project); err != nil {
			return databaseError(err)
		}
	}
	return nil
}

func lockAccounts(ctx context.Context, tx pgx.Tx, organization string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, organization+"/access-users")
	return err
}

func revokeUser(ctx context.Context, tx pgx.Tx, user string) error {
	if _, err := tx.Exec(ctx, `UPDATE access_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, user); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE access_mcp_grants SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, user)
	return err
}
