package store

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func refreshPlacements(ctx context.Context, tx pgx.Tx, policy cloud.Policy) error {
	rows, err := tx.Query(ctx, `SELECT intent FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 ORDER BY id`, policy.OrganizationID, policy.ProjectID)
	if err != nil {
		return err
	}
	items := map[string]cloud.Intent{}
	environments := map[string]bool{}
	for rows.Next() {
		var data []byte
		var item cloud.Intent
		if err := rows.Scan(&data); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(data, &item); err != nil {
			rows.Close()
			return err
		}
		items[item.ID] = item
		if item.Binding != nil {
			environments[item.Binding.EnvironmentID] = true
		}
		if item.Domain != nil {
			environments[item.Domain.Request.EnvironmentID] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for environment := range environments {
		bindings := map[string]v1.WorkloadCloud{}
		domains := []v1.DomainRoute{}
		for _, item := range items {
			if item.Action != "reconcile" || item.Observation == nil {
				continue
			}
			if item.Binding != nil && item.Binding.EnvironmentID == environment {
				binding := item.Binding
				resource := items[binding.ResourceID]
				if resource.Resource == nil || resource.Action != "reconcile" {
					continue
				}
				conn := items[resource.Resource.ConnectionID]
				if conn.Connection == nil || !conn.Connection.Enabled || conn.Action != "reconcile" || conn.Generation != binding.ConnectionGeneration || conn.Generation != resource.Resource.ConnectionGeneration {
					continue
				}
				if item.Observation.State != "ready" && item.Observation.Details["identityReady"] != "true" {
					continue
				}
				for _, consumer := range binding.Consumers {
					value := bindings[consumer.Component]
					value.ServiceAccount = consumer.ServiceAccount
					value.ConnectionID = conn.ID
					value.ConnectionGeneration = conn.Generation
					value.RoleARN = consumer.RoleARN
					value.Region = conn.Connection.Region
					value.EgressCIDRs = policy.EgressCIDRs
					if value.Stores == nil {
						value.Stores = map[string]v1.ObjectStoreBinding{}
					}
					value.Stores[resource.ID] = v1.ObjectStoreBinding{Bucket: resource.Resource.ExternalName, Prefix: consumer.Prefix, Access: binding.Access}
					bindings[consumer.Component] = value
				}
			}
			if item.Domain != nil && item.Domain.Request.EnvironmentID == environment {
				claim := item.Domain
				conn := items[claim.Request.ConnectionID]
				if conn.Connection == nil || !conn.Connection.Enabled || conn.Action != "reconcile" || conn.Generation != claim.ConnectionGeneration || !claim.OwnershipVerified || !claim.DNSReady {
					continue
				}
				domains = append(domains, v1.DomainRoute{ID: item.ID, ConnectionID: conn.ID, ConnectionGeneration: conn.Generation, Component: claim.Request.Component, Hostname: claim.Request.Hostname, IssuerRef: policy.Profiles[claim.Request.Profile].IssuerRef})
			}
		}
		slices.SortFunc(domains, func(a, b v1.DomainRoute) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		if err := postgres.ReconfigureCapabilities(ctx, tx, policy.Scope, environment, bindings, domains); err != nil {
			return err
		}
	}
	return nil
}
