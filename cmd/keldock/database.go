package main

import (
	"context"
	"fmt"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/lucas-ht/keldock/internal/database"
	databasestore "github.com/lucas-ht/keldock/internal/database/store"
)

func configureDatabases(ctx context.Context, api *controlplane.API, settings controlSettings, users *access.Repository) error {
	policies := map[string]database.Policy{}
	for _, policy := range settings.Databases {
		project, ok := api.Projects[policy.ProjectID]
		if !ok || project.Scope != policy.Scope || policy.ClusterID != settings.Cluster || policy.Validate() != nil || policies[policy.ProjectID].ProjectID != "" {
			return fmt.Errorf("invalid or duplicate operator database policy")
		}
		policies[policy.ProjectID] = policy
	}
	// Removing delegation disables new allocations; existing data remains inventoried.
	if _, err := api.Store.Pool().Exec(ctx, `DELETE FROM database_profiles WHERE organization_id=$1`, settings.Organization); err != nil {
		return err
	}
	for _, policy := range policies {
		if err := api.Store.SeedDatabasePolicy(ctx, policy); err != nil {
			return err
		}
	}
	if len(policies) == 0 {
		return nil
	}
	identity, err := clusteridentity.NewDatabaseRepository(api.Store.Pool(), users, settings.Cluster)
	if err != nil {
		return err
	}
	if err := identity.Seed(ctx, settings.Cluster); err != nil {
		return err
	}
	api.Security.Database = identity
	api.Databases = &databasestore.Store{Pool: api.Store.Pool(), Users: users, Policies: policies}
	return nil
}
