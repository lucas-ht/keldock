package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
)

// SeedDatabasePolicy records explicit operator delegation; absence disables allocation.
func (store *Store) SeedDatabasePolicy(ctx context.Context, policy database.Policy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	_, err = store.pool.Exec(ctx, `INSERT INTO database_profiles VALUES($1,$2,$3) ON CONFLICT(organization_id,project_id) DO UPDATE SET configuration=EXCLUDED.configuration`, policy.OrganizationID, policy.ProjectID, data)
	return err
}

func resolveDatabases(ctx context.Context, tx pgx.Tx, scope v1.Scope, release string, selection *v1.Placement) error {
	if selection.Databases != nil {
		return nil
	} // Trusted lifecycle changes already resolved this exact release.
	var data []byte
	if err := tx.QueryRow(ctx, `SELECT input FROM releases WHERE id=$1 AND organization_id=$2 AND project_id=$3`, release, scope.OrganizationID, scope.ProjectID).Scan(&data); err != nil {
		return err
	}
	var input v1.SubmitRelease
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}
	if len(input.Definition.Resources) == 0 {
		return retainDatabases(ctx, tx, selection.EnvironmentID)
	}
	err := tx.QueryRow(ctx, `SELECT configuration FROM database_profiles WHERE organization_id=$1 AND project_id=$2 FOR UPDATE`, scope.OrganizationID, scope.ProjectID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return err
	}
	var policy database.Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	selection.Databases = map[string]v1.DatabaseBinding{}
	aliases := make([]string, 0, len(input.Definition.Resources))
	for name := range input.Definition.Resources {
		aliases = append(aliases, name)
	}
	slices.Sort(aliases)
	// Removing a declaration removes consumer authority, while keeping the data.
	if _, err := tx.Exec(ctx, `UPDATE database_allocations SET generation=generation+1,intent=jsonb_set(jsonb_set(jsonb_set(intent,'{action}','"retain"'),'{components}','[]'),'{generation}',to_jsonb(generation+1)),observation=NULL,updated_at=now() WHERE environment_id=$1 AND NOT(alias=ANY($2::text[])) AND intent->>'action'='reconcile'`, selection.EnvironmentID, aliases); err != nil {
		return err
	}
	for _, alias := range aliases {
		request := input.Definition.Resources[alias]
		profile, ok := policy.Profiles[request.Profile]
		if !ok {
			return ErrDenied
		}
		components := []string{}
		for name, component := range input.Definition.Components {
			if slices.Contains(component.Resources, alias) {
				components = append(components, name)
			}
		}
		slices.Sort(components)
		id := database.ID(selection.EnvironmentID, alias)
		allocation := database.Allocation{Scope: scope, ID: id, EnvironmentID: selection.EnvironmentID, Alias: alias, ClusterID: policy.ClusterID, Namespace: "kd-" + strings.TrimPrefix(id, "db-"), Profile: request.Profile, Configuration: profile, Generation: 1, Action: "reconcile", Components: components}
		var previous []byte
		err = tx.QueryRow(ctx, `SELECT intent FROM database_allocations WHERE organization_id=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, scope.OrganizationID, scope.ProjectID, id).Scan(&previous)
		if err == nil {
			var existing database.Allocation
			if err := json.Unmarshal(previous, &existing); err != nil {
				return err
			}
			if existing.Action != "reconcile" || existing.Profile != request.Profile || existing.Configuration != profile {
				return ErrConflict
			}
			allocation.Generation = existing.Generation
			if !slices.Equal(existing.Components, components) {
				allocation.Generation++
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			var total int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM database_allocations WHERE organization_id=$1 AND project_id=$2`, scope.OrganizationID, scope.ProjectID).Scan(&total); err != nil {
				return err
			}
			if total >= policy.Limit {
				return ErrQuota
			}
		} else {
			return err
		}
		encoded, err := json.Marshal(allocation)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO database_allocations(organization_id,project_id,id,environment_id,alias,generation,intent) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(organization_id,project_id,id) DO UPDATE SET generation=EXCLUDED.generation,intent=EXCLUDED.intent,observation=CASE WHEN database_allocations.generation=EXCLUDED.generation THEN database_allocations.observation ELSE NULL END`, scope.OrganizationID, scope.ProjectID, id, selection.EnvironmentID, alias, allocation.Generation, encoded); err != nil {
			return err
		}
		selection.Databases[alias] = allocation.Binding()
	}
	return nil
}

func retainDatabases(ctx context.Context, tx pgx.Tx, environment string) error {
	_, err := tx.Exec(ctx, `UPDATE database_allocations SET generation=generation+1,intent=jsonb_set(jsonb_set(jsonb_set(intent,'{action}','"retain"'),'{components}','[]'),'{generation}',to_jsonb(generation+1)),observation=NULL,updated_at=now() WHERE environment_id=$1 AND intent->>'action'='reconcile'`, environment)
	return err
}
