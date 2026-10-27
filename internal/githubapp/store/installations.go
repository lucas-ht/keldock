package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
)

// Installation is a safe snapshot of external delegation and a local disconnect fence.
type Installation struct {
	ID          int64             `json:"id"`
	AccountID   int64             `json:"accountId"`
	Login       string            `json:"login"`
	Active      bool              `json:"active"`
	Blocked     bool              `json:"blocked"`
	Permissions map[string]string `json:"permissions"`
}

// RepositoryGrant binds a configured project to a stable, currently selected GitHub repository.
type RepositoryGrant struct {
	ID             int64  `json:"id"`
	InstallationID int64  `json:"installationId"`
	OwnerID        int64  `json:"ownerId"`
	FullName       string `json:"fullName"`
	ProjectID      string `json:"projectId"`
	Active         bool   `json:"active"`
}

// Snapshot contains only a configured owner's verified remote installation inventory.
type Snapshot struct {
	Installation client.Installation
	Repositories []RepositoryGrant
}

// TrackedPulls bounds recovery to open PRs previously observed for this configured project.
func (repository *Repository) TrackedPulls(ctx context.Context, project string) ([]int64, error) {
	rows, err := repository.pool.Query(ctx, `SELECT number FROM pull_requests WHERE organization_id=$1 AND project_id=$2 AND NOT closed ORDER BY number LIMIT 1001`, repository.Organization, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []int64{}
	for rows.Next() {
		var number int64
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		result = append(result, number)
	}
	if len(result) > 1000 {
		return nil, access.ErrLimited
	}
	return result, rows.Err()
}

// ReplaceSnapshot fences omitted/revoked delegations in the same transaction as new inventory.
func (repository *Repository) ReplaceSnapshot(ctx context.Context, app client.App, fingerprint string, snapshots []Snapshot) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var enabled bool
	var id int64
	var stored string
	if err := tx.QueryRow(ctx, `SELECT app_id,enabled,fingerprint FROM github_apps WHERE organization_id=$1 FOR UPDATE`, repository.Organization).Scan(&id, &enabled, &stored); err != nil {
		return err
	}
	if !enabled || id != app.ID || stored != fingerprint {
		return access.ErrDenied
	}
	if _, err := tx.Exec(ctx, `UPDATE github_installations SET active=false WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE github_repositories SET active=false WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		item := snapshot.Installation
		permissions, err := json.Marshal(item.Permissions)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO github_installations (organization_id,id,account_id,login,active,permissions) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (organization_id,id) DO UPDATE SET account_id=EXCLUDED.account_id,login=EXCLUDED.login,active=EXCLUDED.active,permissions=EXCLUDED.permissions,updated_at=now()`, repository.Organization, item.ID, item.Account.ID, item.Account.Login, item.SuspendedAt == nil, permissions); err != nil {
			return err
		}
		for _, grant := range snapshot.Repositories {
			if _, err := tx.Exec(ctx, `INSERT INTO github_repositories (organization_id,id,installation_id,owner_id,full_name,project_id,active) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (organization_id,id) DO UPDATE SET installation_id=EXCLUDED.installation_id,owner_id=EXCLUDED.owner_id,full_name=EXCLUDED.full_name,project_id=EXCLUDED.project_id,active=EXCLUDED.active,updated_at=now()`, repository.Organization, grant.ID, item.ID, grant.OwnerID, grant.FullName, grant.ProjectID, item.SuspendedAt == nil); err != nil {
				return err
			}
		}
	}
	permissions, err := json.Marshal(app.Permissions)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE github_apps SET permissions=$2,synced_at=now(),sync_error='' WHERE organization_id=$1`, repository.Organization, permissions); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Inventory is the last verified inventory, including visibly disconnected entries.
func (repository *Repository) Inventory(ctx context.Context) ([]Installation, []RepositoryGrant, error) {
	installations := []Installation{}
	grants := []RepositoryGrant{}
	rows, err := repository.pool.Query(ctx, `SELECT id,account_id,login,active,blocked,permissions FROM github_installations WHERE organization_id=$1 ORDER BY id LIMIT 1000`, repository.Organization)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var item Installation
		if err := rows.Scan(&item.ID, &item.AccountID, &item.Login, &item.Active, &item.Blocked, &item.Permissions); err != nil {
			rows.Close()
			return nil, nil, err
		}
		installations = append(installations, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = repository.pool.Query(ctx, `SELECT r.id,r.installation_id,r.owner_id,r.full_name,r.project_id,(r.active AND i.active AND NOT i.blocked AND a.enabled) FROM github_repositories r JOIN github_installations i ON i.organization_id=r.organization_id AND i.id=r.installation_id JOIN github_apps a ON a.organization_id=r.organization_id WHERE r.organization_id=$1 ORDER BY r.full_name LIMIT 1000`, repository.Organization)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item RepositoryGrant
		if err := rows.Scan(&item.ID, &item.InstallationID, &item.OwnerID, &item.FullName, &item.ProjectID, &item.Active); err != nil {
			return nil, nil, err
		}
		grants = append(grants, item)
	}
	return installations, grants, rows.Err()
}

// Grant rechecks live local delegation; an expired inventory cannot authorize new CI work.
func (repository *Repository) Grant(ctx context.Context, id int64) (RepositoryGrant, error) {
	var grant RepositoryGrant
	err := repository.pool.QueryRow(ctx, `SELECT r.id,r.installation_id,r.owner_id,r.full_name,r.project_id,true FROM github_repositories r
JOIN github_installations i ON i.organization_id=r.organization_id AND i.id=r.installation_id JOIN github_apps a ON a.organization_id=r.organization_id
WHERE r.organization_id=$1 AND r.id=$2 AND r.active AND i.active AND NOT i.blocked AND a.enabled AND a.synced_at>now()-interval '5 minutes'`, repository.Organization, id).Scan(&grant.ID, &grant.InstallationID, &grant.OwnerID, &grant.FullName, &grant.ProjectID, &grant.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return grant, access.ErrDenied
	}
	return grant, err
}

// BlockInstallation changes local delegation with current administrator authority and CAS.
func (repository *Repository) BlockInstallation(ctx context.Context, principal access.Principal, expected, id int64, blocked bool) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	user, err := repository.administrator(ctx, tx, principal)
	if err != nil {
		return err
	}
	if err := repository.version(ctx, tx, expected); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE github_installations SET blocked=$3,updated_at=$4 WHERE organization_id=$1 AND id=$2`, repository.Organization, id, blocked, time.Now())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return access.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE github_apps SET version=version+1 WHERE organization_id=$1`, repository.Organization); err != nil {
		return err
	}
	event := "github.installation_connected"
	if blocked {
		event = "github.installation_disconnected"
	}
	if err := access.RecordAudit(ctx, tx, repository.Organization, "user:"+user.ID, event, "installation"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
