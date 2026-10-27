package access

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

// Users lists one bounded page for a current human administrator.
func (repository *Repository) Users(ctx context.Context, principal Principal, after string) ([]User, string, error) {
	if !principal.IsAdmin() || principal.User.OrganizationID != repository.Organization {
		return nil, "", ErrDenied
	}
	rows, err := repository.pool.Query(ctx, `SELECT `+userColumns+` FROM access_users u WHERE u.organization_id=$1 AND u.id>$2 ORDER BY u.id LIMIT 51`, repository.Organization, after)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, "", err
		}
		users = append(users, user)
	}
	var next string
	if len(users) > 50 {
		users = users[:50]
		next = users[49].ID
	}
	return users, next, rows.Err()
}

// UpdateUser prevents stale edits and removal of the last active administrator.
// Every account revision invalidates prior sessions and MCP grants.
func (repository *Repository) UpdateUser(ctx context.Context, principal Principal, id string, input UserUpdate) (User, error) {
	if !principal.IsAdmin() || !slices.Contains([]string{"active", "invited", "disabled"}, input.Status) {
		return User{}, ErrDenied
	}
	_, name, err := accountFields(principal.User.Email, input.Name, input.Role, input.Projects)
	if err != nil {
		return User{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return User{}, err
	}
	actor, err := repository.RequireHuman(ctx, tx, principal)
	if err != nil || actor.Role != "admin" {
		return User{}, ErrDenied
	}
	var role, status string
	var version int64
	var passwordSet bool
	if err := tx.QueryRow(ctx, `SELECT role,status,version,password_hash<>'' FROM access_users WHERE organization_id=$1 AND id=$2 FOR UPDATE`, repository.Organization, id).Scan(&role, &status, &version, &passwordSet); err != nil {
		return User{}, databaseError(err)
	}
	if version != input.Version || (input.Status == "invited" && status != "invited") || (input.Status == "active" && !passwordSet) {
		return User{}, ErrConflict
	}
	if role == "admin" && status == "active" && (input.Role != "admin" || input.Status != "active") {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM access_users WHERE organization_id=$1 AND role='admin' AND status='active'`, repository.Organization).Scan(&count); err != nil {
			return User{}, err
		}
		if count <= 1 {
			return User{}, ErrConflict
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE access_users SET name=$2,role=$3,status=$4,version=version+1,updated_at=now() WHERE id=$1`, id, name, input.Role, input.Status); err != nil {
		return User{}, err
	}
	if err := replaceProjects(ctx, tx, repository.Organization, id, input.Projects); err != nil {
		return User{}, err
	}
	if err := revokeUser(ctx, tx, id); err != nil {
		return User{}, err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "user:"+actor.ID, "user.updated", id); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return repository.User(ctx, id)
}

// PasswordAccount is an internal authentication record and must never be serialized.
type PasswordAccount struct {
	User User   `json:"-"`
	Hash string `json:"-"`
}

// PasswordAccount retrieves one account for constant-cost password verification.
func (repository *Repository) PasswordAccount(ctx context.Context, email string) (PasswordAccount, error) {
	var account PasswordAccount
	var id string
	if err := repository.pool.QueryRow(ctx, `SELECT id,password_hash FROM access_users WHERE organization_id=$1 AND email=$2`, repository.Organization, email).Scan(&id, &account.Hash); err != nil {
		return account, databaseError(err)
	}
	user, err := repository.User(ctx, id)
	account.User = user
	return account, err
}

// ChangePassword rotates credentials and invalidates all older account authority.
func (repository *Repository) ChangePassword(ctx context.Context, principal Principal, oldHash, newHash string) (string, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return "", err
	}
	if _, err := repository.RequireHuman(ctx, tx, principal); err != nil {
		return "", err
	}
	result, err := tx.Exec(ctx, `UPDATE access_users SET password_hash=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND password_hash=$3 AND status='active'`, repository.Organization, principal.User.ID, oldHash, newHash)
	if err != nil {
		return "", err
	}
	if result.RowsAffected() != 1 {
		return "", ErrConflict
	}
	if err := revokeUser(ctx, tx, principal.User.ID); err != nil {
		return "", err
	}
	user, err := LoadUser(ctx, tx, repository.Organization, principal.User.ID)
	if err != nil {
		return "", err
	}
	token, err := insertSession(ctx, tx, user)
	if err != nil {
		return "", err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "user:"+user.ID, "password.changed", user.ID); err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}

// recoveryTarget is restricted to the operator's private recovery command.
func (repository *Repository) recoveryTarget(ctx context.Context, tx pgx.Tx, email string) (User, error) {
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM access_users WHERE organization_id=$1 AND email=$2 AND status='active' FOR UPDATE`, repository.Organization, email).Scan(&id); err != nil {
		return User{}, databaseError(err)
	}
	return LoadUser(ctx, tx, repository.Organization, id)
}
