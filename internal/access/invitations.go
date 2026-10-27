package access

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Bootstrap creates the first administrator invitation through an operator-only CLI.
// Repeating it for the sole unactivated administrator replaces a lost/expired invitation.
func (repository *Repository) Bootstrap(ctx context.Context, email, name string) (Invitation, error) {
	email, name, err := accountFields(email, name, "admin", nil)
	if err != nil {
		return Invitation{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return Invitation{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM access_users WHERE organization_id=$1`, repository.Organization).Scan(&count); err != nil {
		return Invitation{}, err
	}
	id := NewID("usr")
	if count > 0 {
		if count != 1 {
			return Invitation{}, ErrConflict
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM access_users WHERE organization_id=$1 AND email=$2 AND role='admin' AND status='invited'`, repository.Organization, email).Scan(&id); err != nil {
			return Invitation{}, ErrConflict
		}
	} else if _, err := tx.Exec(ctx, `INSERT INTO access_users (id,organization_id,email,name,role,status) VALUES ($1,$2,$3,$4,'admin','invited')`, id, repository.Organization, email, name); err != nil {
		return Invitation{}, databaseError(err)
	}
	invitation, err := repository.insertInvitation(ctx, tx, id, "activate", "operator-cli")
	if err != nil {
		return Invitation{}, err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "operator-cli", "administrator.bootstrap", id); err != nil {
		return Invitation{}, err
	}
	return invitation, tx.Commit(ctx)
}

// Invite creates an account and a bounded single-use activation grant without sending email.
func (repository *Repository) Invite(ctx context.Context, principal Principal, email, name, role string, projects []string) (Invitation, error) {
	if !principal.IsAdmin() {
		return Invitation{}, ErrDenied
	}
	email, name, err := accountFields(email, name, role, projects)
	if err != nil {
		return Invitation{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return Invitation{}, err
	}
	actor, err := repository.RequireHuman(ctx, tx, principal)
	if err != nil || actor.Role != "admin" {
		return Invitation{}, ErrDenied
	}
	id := NewID("usr")
	if _, err := tx.Exec(ctx, `INSERT INTO access_users (id,organization_id,email,name,role,status) VALUES ($1,$2,$3,$4,$5,'invited')`, id, repository.Organization, email, name, role); err != nil {
		return Invitation{}, databaseError(err)
	}
	if err := replaceProjects(ctx, tx, repository.Organization, id, projects); err != nil {
		return Invitation{}, err
	}
	invitation, err := repository.insertInvitation(ctx, tx, id, "activate", "user:"+actor.ID)
	if err != nil {
		return Invitation{}, err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "user:"+actor.ID, "user.invited", id); err != nil {
		return Invitation{}, err
	}
	return invitation, tx.Commit(ctx)
}

func (repository *Repository) insertInvitation(ctx context.Context, tx pgx.Tx, id, kind, actor string) (Invitation, error) {
	if _, err := tx.Exec(ctx, `UPDATE access_invitations SET consumed_at=now() WHERE user_id=$1 AND consumed_at IS NULL`, id); err != nil {
		return Invitation{}, err
	}
	invitation := Invitation{Token: NewToken("kdi_")}
	if err := tx.QueryRow(ctx, `INSERT INTO access_invitations (token_hash,user_id,kind,expires_at,created_by) VALUES ($1,$2,$3,now()+interval '48 hours',$4) RETURNING expires_at`, TokenHash(invitation.Token), id, kind, actor).Scan(&invitation.ExpiresAt); err != nil {
		return Invitation{}, err
	}
	user, err := LoadUser(ctx, tx, repository.Organization, id)
	invitation.User = user
	return invitation, err
}

// ReissueInvitation replaces an activation link or resets an active account's password.
// Resetting immediately invalidates older sessions and MCP grants.
func (repository *Repository) ReissueInvitation(ctx context.Context, principal Principal, id string) (Invitation, error) {
	if !principal.IsAdmin() {
		return Invitation{}, ErrDenied
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return Invitation{}, err
	}
	actor, err := repository.RequireHuman(ctx, tx, principal)
	if err != nil || actor.Role != "admin" {
		return Invitation{}, ErrDenied
	}
	user, err := LoadUser(ctx, tx, repository.Organization, id)
	if err != nil {
		return Invitation{}, err
	}
	if user.Status == "disabled" {
		return Invitation{}, ErrConflict
	}
	kind := "activate"
	if user.Status == "active" {
		kind = "reset"
		if _, err := tx.Exec(ctx, `UPDATE access_users SET version=version+1,updated_at=now() WHERE id=$1`, id); err != nil {
			return Invitation{}, err
		}
		if err := revokeUser(ctx, tx, id); err != nil {
			return Invitation{}, err
		}
	}
	invitation, err := repository.insertInvitation(ctx, tx, id, kind, "user:"+actor.ID)
	if err != nil {
		return Invitation{}, err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "user:"+actor.ID, "invitation.reissued", id); err != nil {
		return Invitation{}, err
	}
	return invitation, tx.Commit(ctx)
}

// InvitationInfo reveals only the intended recipient to a valid invitation holder.
func (repository *Repository) InvitationInfo(ctx context.Context, token string) (Invitation, string, error) {
	var id, kind string
	var expires time.Time
	err := repository.pool.QueryRow(ctx, `SELECT i.user_id,i.kind,i.expires_at FROM access_invitations i JOIN access_users u ON u.id=i.user_id WHERE i.token_hash=$1 AND u.organization_id=$2 AND i.consumed_at IS NULL AND i.expires_at>now() AND u.status IN ('active','invited')`, TokenHash(token), repository.Organization).Scan(&id, &kind, &expires)
	if err != nil {
		return Invitation{}, "", ErrDenied
	}
	user, err := repository.User(ctx, id)
	return Invitation{User: user, ExpiresAt: expires}, kind, err
}

// Activate consumes an invitation exactly once and returns a newly rotated browser session.
func (repository *Repository) Activate(ctx context.Context, token, passwordHash string) (User, string, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return User{}, "", err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return User{}, "", err
	}
	var id, kind, status string
	err = tx.QueryRow(ctx, `SELECT i.user_id,i.kind,u.status FROM access_invitations i JOIN access_users u ON u.id=i.user_id WHERE i.token_hash=$1 AND u.organization_id=$2 AND i.consumed_at IS NULL AND i.expires_at>now() FOR UPDATE OF i,u`, TokenHash(token), repository.Organization).Scan(&id, &kind, &status)
	if err != nil || (kind == "activate" && status != "invited") || (kind == "reset" && status != "active") {
		return User{}, "", ErrDenied
	}
	if _, err := tx.Exec(ctx, `UPDATE access_invitations SET consumed_at=now() WHERE token_hash=$1`, TokenHash(token)); err != nil {
		return User{}, "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_users SET password_hash=$2,status='active',version=version+1,updated_at=now(),last_login_at=now() WHERE id=$1`, id, passwordHash); err != nil {
		return User{}, "", err
	}
	if err := revokeUser(ctx, tx, id); err != nil {
		return User{}, "", err
	}
	user, err := LoadUser(ctx, tx, repository.Organization, id)
	if err != nil {
		return User{}, "", err
	}
	session, err := insertSession(ctx, tx, user)
	if err != nil {
		return User{}, "", err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "user:"+id, "invitation.accepted", id); err != nil {
		return User{}, "", err
	}
	return user, session, tx.Commit(ctx)
}

// Recover resets an active account using operator-held database authority, without a paid service.
func (repository *Repository) Recover(ctx context.Context, email, passwordHash string) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if err := lockAccounts(ctx, tx, repository.Organization); err != nil {
		return err
	}
	user, err := repository.recoveryTarget(ctx, tx, email)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_users SET password_hash=$2,version=version+1,updated_at=now() WHERE id=$1`, user.ID, passwordHash); err != nil {
		return err
	}
	if err := revokeUser(ctx, tx, user.ID); err != nil {
		return err
	}
	if err := RecordAudit(ctx, tx, repository.Organization, "operator-cli", "password.recovered", user.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
