// Package store persists authorized cloud intent and evidence without holding AWS credentials.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Store has project-specific operator configuration; absent profiles remain disabled.
type Store struct {
	Pool     *pgxpool.Pool
	Users    *access.Repository
	Policies map[string]cloud.Policy
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Debug("cloud transaction rollback", "error", err)
	}
}

func (store *Store) policy(principal access.Principal, project string, mutate, admin bool) (cloud.Policy, error) {
	policy, ok := store.Policies[project]
	if !ok || principal.User.OrganizationID != policy.OrganizationID || !principal.CanRead(project) || (mutate && (principal.Kind != "session" || !principal.CanOperate(project))) || (admin && !principal.IsAdmin()) {
		return cloud.Policy{}, access.ErrDenied
	}
	return policy, policy.Validate()
}

func get(ctx context.Context, tx pgx.Tx, scope v1.Scope, id string) (cloud.Intent, error) {
	var result cloud.Intent
	var data []byte
	err := tx.QueryRow(ctx, `SELECT intent FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, scope.OrganizationID, scope.ProjectID, id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, postgres.ErrNotFound
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

func save(ctx context.Context, tx pgx.Tx, policy cloud.Policy, item cloud.Intent) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	externalKey := ""
	if item.Domain != nil && item.Action != "disconnected" {
		externalKey = item.Domain.Request.Hostname
	}
	_, err = tx.Exec(ctx, `INSERT INTO cloud_intents(organization_id,project_id,cluster_id,id,kind,generation,intent,external_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(organization_id,project_id,id) DO UPDATE SET generation=EXCLUDED.generation,intent=EXCLUDED.intent,external_key=EXCLUDED.external_key,updated_at=now()`, policy.OrganizationID, policy.ProjectID, policy.ClusterID, item.ID, item.Kind, item.Generation, data, externalKey)
	var constraint interface{ SQLState() string }
	if errors.As(err, &constraint) && constraint.SQLState() == "23505" {
		return postgres.ErrConflict
	}
	return err
}

// List returns a bounded project inventory, including retained and disconnected entries.
func (store *Store) List(ctx context.Context, principal access.Principal, project string) ([]cloud.Intent, error) {
	policy, err := store.policy(principal, project, false, false)
	if err != nil {
		return nil, err
	}
	return store.list(ctx, policy.OrganizationID, policy.ClusterID, project)
}

// Desired is available only behind the distinct cloud-agent authenticator.
func (store *Store) Desired(ctx context.Context, organization, cluster string) ([]cloud.Intent, error) {
	return store.list(ctx, organization, cluster, "")
}

func (store *Store) list(ctx context.Context, organization, cluster, project string) ([]cloud.Intent, error) {
	rows, err := store.Pool.Query(ctx, `SELECT intent FROM cloud_intents WHERE organization_id=$1 AND cluster_id=$2 AND ($3='' OR project_id=$3) ORDER BY id LIMIT 1001`, organization, cluster, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []cloud.Intent{}
	for rows.Next() {
		var data []byte
		var item cloud.Intent
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if len(items) > 1000 {
		return nil, postgres.ErrLimited
	}
	return items, rows.Err()
}
