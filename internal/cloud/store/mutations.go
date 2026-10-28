package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Mutate authorizes and commits intent, receipt and audit together. A lost response is replayable.
func (store *Store) Mutate(ctx context.Context, principal access.Principal, project, key string, command cloud.Command) (cloud.Receipt, error) {
	var receipt cloud.Receipt
	policy, err := store.policy(principal, project, true, command.Kind == "connection")
	if err != nil {
		return receipt, err
	}
	if len(key) < 8 || len(key) > 128 || (command.ID != "" && !v1.ValidName(command.ID)) {
		return receipt, access.ErrInvalid
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer rollback(ctx, tx)
	user, err := store.Users.RequireHuman(ctx, tx, principal)
	if err != nil {
		return receipt, err
	}
	principal.User = user
	if _, err := store.policy(principal, project, true, command.Kind == "connection"); err != nil {
		return receipt, err
	}
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE organization_id=$1 AND id=$2 AND cluster_id=$3 FOR UPDATE`, policy.OrganizationID, project, policy.ClusterID).Scan(&locked); err != nil {
		return receipt, err
	}
	hash, err := provenance.Hash(struct {
		Actor   string
		Command cloud.Command
	}{user.ID, command})
	if err != nil {
		return receipt, err
	}
	var previous string
	var data []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response FROM cloud_receipts WHERE organization_id=$1 AND project_id=$2 AND key=$3`, policy.OrganizationID, project, key).Scan(&previous, &data)
	if err == nil {
		if previous != hash {
			return receipt, postgres.ErrConflict
		}
		if err := json.Unmarshal(data, &receipt); err != nil {
			return receipt, err
		}
		return receipt, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	item, err := store.command(ctx, tx, policy, principal, command)
	if err != nil {
		return receipt, err
	}
	if err := save(ctx, tx, policy, item); err != nil {
		return receipt, err
	}
	receipt = cloud.Receipt{APIVersion: v1.APIVersion, ID: item.ID, Generation: item.Generation, StatusURL: "/v1/projects/" + project + "/cloud/intents/" + item.ID}
	data, err = json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO cloud_receipts VALUES($1,$2,$3,$4,$5)`, policy.OrganizationID, project, key, hash, data); err != nil {
		return receipt, err
	}
	if err := access.RecordAudit(ctx, tx, policy.OrganizationID, "user:"+user.ID, "cloud."+command.Kind, item.ID); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}

func (store *Store) command(ctx context.Context, tx pgx.Tx, policy cloud.Policy, principal access.Principal, command cloud.Command) (cloud.Intent, error) {
	id := command.ID
	if id == "" {
		id = access.NewID("cloud")
	}
	item := cloud.Intent{Scope: policy.Scope, ID: id, Kind: command.Kind, Generation: 1, Action: "reconcile", State: "pending", UpdatedAt: time.Now().UTC()}
	if command.Kind != "connection" && command.Kind != "lifecycle" {
		if _, err := get(ctx, tx, policy.Scope, id); !errors.Is(err, postgres.ErrNotFound) {
			if err == nil {
				return item, postgres.ErrConflict
			}
			return item, err
		}
	}
	switch command.Kind {
	case "connection":
		return store.connection(ctx, tx, policy, item, command.Input)
	case "resource":
		return store.resource(ctx, tx, policy, item, command.Input)
	case "binding":
		return store.binding(ctx, tx, policy, item, command.Input)
	case "domain":
		return store.domain(ctx, tx, policy, item, command.Input)
	case "backup":
		return store.backup(ctx, tx, policy, item, command.Input)
	case "lifecycle":
		if command.ID == "" {
			return item, access.ErrInvalid
		}
		return store.lifecycle(ctx, tx, policy, principal, command.ID, command.Input)
	default:
		return item, access.ErrInvalid
	}
}

func connection(ctx context.Context, tx pgx.Tx, policy cloud.Policy, id string) (cloud.Intent, error) {
	item, err := get(ctx, tx, policy.Scope, id)
	if err != nil {
		return item, err
	}
	if item.Connection == nil || !item.Connection.Enabled || item.Action != "reconcile" || policy.ValidateConnection(*item.Connection) != nil {
		return item, access.ErrDenied
	}
	if item.Observation == nil || item.Observation.Generation != item.Generation || item.Observation.State != "ready" {
		return item, postgres.ErrConflict
	}
	if time.Since(item.Observation.ObservedAt) > 2*time.Minute {
		return item, postgres.ErrStale
	}
	return item, nil
}

func count(ctx context.Context, tx pgx.Tx, policy cloud.Policy, kind, profile string, limit int) error {
	var total int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND kind=$3 AND ($4='' OR intent->'resource'->>'profile'=$4 OR intent->'domain'->'request'->>'profile'=$4)`, policy.OrganizationID, policy.ProjectID, kind, profile).Scan(&total)
	if err != nil {
		return err
	}
	if total >= limit {
		return postgres.ErrQuota
	}
	return nil
}
