package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func inheritCapabilities(ctx context.Context, tx pgx.Tx, scope v1.Scope, environment string, selection *v1.Placement) error {
	var data []byte
	err := tx.QueryRow(ctx, `SELECT selection FROM deployments WHERE environment_id=$1 ORDER BY generation DESC LIMIT 1`, environment).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var previous v1.Placement
	if err := json.Unmarshal(data, &previous); err != nil {
		return err
	}
	if selection.Cloud == nil {
		selection.Cloud = previous.Cloud
	}
	if selection.Domains == nil {
		selection.Domains = previous.Domains
	}
	for component, binding := range selection.Cloud {
		allowed, err := cloudConnectionActive(ctx, tx, scope, binding.ConnectionID, binding.ConnectionGeneration)
		if err != nil {
			return err
		}
		if !allowed {
			delete(selection.Cloud, component)
			continue
		}
		for resource := range binding.Stores {
			var granted bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND kind='binding' AND intent->>'action'='reconcile' AND intent->'binding'->>'environmentId'=$3 AND intent->'binding'->>'resourceId'=$4 AND intent->'binding'->'components' ? $5)`, scope.OrganizationID, scope.ProjectID, environment, resource, component).Scan(&granted); err != nil {
				return err
			}
			if !granted {
				delete(binding.Stores, resource)
			}
		}
		if len(binding.Stores) == 0 {
			delete(selection.Cloud, component)
		} else {
			selection.Cloud[component] = binding
		}
	}
	filtered := []v1.DomainRoute{}
	for _, route := range selection.Domains {
		allowed, err := cloudConnectionActive(ctx, tx, scope, route.ConnectionID, route.ConnectionGeneration)
		if err != nil {
			return err
		}
		var claimed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND id=$3 AND kind='domain' AND intent->>'action'='reconcile')`, scope.OrganizationID, scope.ProjectID, route.ID).Scan(&claimed); err != nil {
			return err
		}
		if allowed && claimed {
			filtered = append(filtered, route)
		}
	}
	selection.Domains = filtered
	return nil
}

func cloudConnectionActive(ctx context.Context, tx pgx.Tx, scope v1.Scope, id string, generation int64) (bool, error) {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND id=$3 AND generation=$4 AND kind='connection' AND intent->>'action'='reconcile' AND intent->'connection'->>'enabled'='true')`, scope.OrganizationID, scope.ProjectID, id, generation).Scan(&allowed)
	return allowed, err
}

// ReconfigureCapabilities selects a new immutable deployment after a trusted binding change.
// It cannot revive a deleted, expired or canceled environment or replace admitted CI input.
func ReconfigureCapabilities(ctx context.Context, tx pgx.Tx, scope v1.Scope, environment string, bindings map[string]v1.WorkloadCloud, domains []v1.DomainRoute) error {
	var data []byte
	var release, state string
	err := tx.QueryRow(ctx, `SELECT d.selection,d.release_id,o.state FROM environments e JOIN deployments d ON d.environment_id=e.id AND d.generation=e.generation JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation WHERE e.id=$1 AND e.organization_id=$2 AND e.project_id=$3 AND NOT e.deleted AND (e.expires_at IS NULL OR e.expires_at>now()) FOR UPDATE OF e,o`, environment, scope.OrganizationID, scope.ProjectID).Scan(&data, &release, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state == "canceled" {
		return nil
	}
	var selection v1.Placement
	if err := json.Unmarshal(data, &selection); err != nil {
		return err
	}
	if len(bindings) == 0 {
		bindings = nil
	}
	if len(domains) == 0 {
		domains = nil
	}
	if reflect.DeepEqual(selection.Cloud, bindings) && reflect.DeepEqual(selection.Domains, domains) {
		return nil
	}
	selection.Cloud = bindings
	if selection.Cloud == nil {
		selection.Cloud = map[string]v1.WorkloadCloud{}
	}
	selection.Domains = domains
	if selection.Domains == nil {
		selection.Domains = []v1.DomainRoute{}
	}
	selection.Generation++
	selection.DeploymentID = newID("dep")
	receipt, err := selectDeployment(ctx, tx, scope, release, selection)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE environments SET generation=$2 WHERE id=$1`, environment, selection.Generation); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(organization_id,project_id,actor,event,target,generation) VALUES($1,$2,'cloud-agent','capabilities.configured',$3,$4)`, scope.OrganizationID, scope.ProjectID, receipt.OperationID, selection.Generation)
	return err
}
