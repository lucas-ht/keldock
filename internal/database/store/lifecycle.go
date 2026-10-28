package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/provenance"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Command selects a reviewable lifecycle transition against one exact generation.
type Command struct {
	ID         string `json:"id"`
	Generation int64  `json:"generation"`
	Action     string `json:"action"`
}

// Mutate retains/disconnects or explicitly reconnects an allocation; it never deletes data.
func (s *Store) Mutate(ctx context.Context, p access.Principal, project, key string, command Command) (database.Allocation, error) {
	var item database.Allocation
	policy, ok := s.Policies[project]
	if !ok || p.Kind != "session" || !p.CanOperate(project) || p.User.OrganizationID != policy.OrganizationID {
		return item, access.ErrDenied
	}
	if len(key) < 16 || len(key) > 128 || !v1.ValidName(command.ID) || command.Generation < 1 || !slices.Contains([]string{"retain", "reconnect"}, command.Action) {
		return item, access.ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return item, err
	}
	defer rollback(ctx, tx)
	user, err := s.Users.RequireHuman(ctx, tx, p)
	if err != nil {
		return item, err
	}
	if !(access.Principal{Kind: "session", User: user}).CanOperate(project) {
		return item, access.ErrDenied
	}
	p.User = user
	var data []byte
	if _, err := tx.Exec(ctx, `SELECT e.id FROM environments e JOIN database_allocations a ON a.environment_id=e.id WHERE a.organization_id=$1 AND a.project_id=$2 AND a.id=$3 FOR UPDATE OF e`, policy.OrganizationID, project, command.ID); err != nil {
		return item, err
	}
	var envData []byte
	var release, state string
	var deleted bool
	err = tx.QueryRow(ctx, `SELECT a.intent,d.selection,d.release_id,o.state,e.deleted FROM database_allocations a JOIN environments e ON e.id=a.environment_id JOIN deployments d ON d.environment_id=e.id AND d.generation=e.generation JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation WHERE a.organization_id=$1 AND a.project_id=$2 AND a.id=$3 FOR UPDATE OF a,e,o`, policy.OrganizationID, project, command.ID).Scan(&data, &envData, &release, &state, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, postgres.ErrNotFound
	}
	if err != nil {
		return item, err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM database_profiles WHERE organization_id=$1 AND project_id=$2 FOR UPDATE`, policy.OrganizationID, project); err != nil {
		return item, err
	}
	hash, err := provenance.Hash(command)
	if err != nil {
		return item, err
	}
	var previous string
	var response []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response FROM database_receipts WHERE organization_id=$1 AND project_id=$2 AND key=$3`, policy.OrganizationID, project, key).Scan(&previous, &response)
	if err == nil {
		if previous != hash {
			return item, postgres.ErrConflict
		}
		if err := json.Unmarshal(response, &item); err != nil {
			return item, err
		}
		return item, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return item, err
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return item, err
	}
	if item.Generation != command.Generation {
		return item, postgres.ErrStale
	}
	var placement v1.Placement
	if err := json.Unmarshal(envData, &placement); err != nil {
		return item, err
	}
	item.Generation++
	item.Observation = nil
	item.Action = "retain"
	item.Components = []string{}
	if command.Action == "reconnect" {
		if !p.IsAdmin() || deleted || state == "canceled" {
			return item, access.ErrDenied
		}
		var input v1.SubmitRelease
		if err := tx.QueryRow(ctx, `SELECT input FROM releases WHERE id=$1 AND organization_id=$2 AND project_id=$3`, release, policy.OrganizationID, project).Scan(&data); err != nil {
			return item, err
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return item, err
		}
		request, exists := input.Definition.Resources[item.Alias]
		if !exists || request.Profile != item.Profile {
			return item, postgres.ErrConflict
		}
		for name, component := range input.Definition.Components {
			if slices.Contains(component.Resources, item.Alias) {
				item.Components = append(item.Components, name)
			}
		}
		slices.Sort(item.Components)
		item.Action = "reconcile"
	}
	data, err = json.Marshal(item)
	if err != nil {
		return item, err
	}
	if _, err := tx.Exec(ctx, `UPDATE database_allocations SET generation=$4,intent=$5,observation=NULL,updated_at=now() WHERE organization_id=$1 AND project_id=$2 AND id=$3`, policy.OrganizationID, project, item.ID, item.Generation, data); err != nil {
		return item, err
	}
	if placement.Databases == nil {
		placement.Databases = map[string]v1.DatabaseBinding{}
	}
	delete(placement.Databases, item.Alias)
	if item.Action == "reconcile" {
		placement.Databases[item.Alias] = item.Binding()
	}
	if !deleted && state != "canceled" {
		if err := postgres.ReconfigureDatabases(ctx, tx, policy.Scope, release, placement); err != nil {
			return item, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO database_receipts VALUES($1,$2,$3,$4,$5)`, policy.OrganizationID, project, key, hash, data); err != nil {
		return item, err
	}
	if err := access.RecordAudit(ctx, tx, policy.OrganizationID, "user:"+p.User.ID, "database."+strings.TrimSpace(command.Action), item.ID); err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}
