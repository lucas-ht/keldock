package store

import (
	"context"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Observe accepts current evidence only from the independently authenticated cloud agent.
func (store *Store) Observe(ctx context.Context, organization, cluster string, report cloud.Evidence) error {
	if !v1.ValidName(report.ID) || report.Generation < 1 || len(report.Reason) > 128 || len(report.Details) > 32 || report.ObservedAt.Before(time.Now().Add(-5*time.Minute)) || report.ObservedAt.After(time.Now().Add(time.Minute)) {
		return access.ErrInvalid
	}
	for key, value := range report.Details {
		if len(key) > 64 || len(value) > 512 {
			return access.ErrInvalid
		}
	}
	switch report.State {
	case "ready", "reconciling", "failed", "suspended", "retained", "disconnected":
	default:
		return access.ErrInvalid
	}
	var project string
	if err := store.Pool.QueryRow(ctx, `SELECT project_id FROM cloud_intents WHERE organization_id=$1 AND cluster_id=$2 AND id=$3`, organization, cluster, report.ID).Scan(&project); err != nil {
		return postgres.ErrNotFound
	}
	policy, ok := store.Policies[project]
	if !ok || policy.OrganizationID != organization || policy.ClusterID != cluster {
		return access.ErrDenied
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organization, project).Scan(&locked); err != nil {
		return err
	}
	item, err := get(ctx, tx, policy.Scope, report.ID)
	if err != nil {
		return err
	}
	if item.Generation != report.Generation || (item.Observation != nil && !report.ObservedAt.After(item.Observation.ObservedAt)) {
		return postgres.ErrStale
	}
	if item.Connection != nil {
		if item.Connection.Enabled && report.State == "ready" && report.Details["account"] != item.Connection.Account {
			return access.ErrDenied
		}
		if !item.Connection.Enabled && report.State == "ready" {
			return postgres.ErrStale
		}
	} else {
		var connID string
		switch {
		case item.Resource != nil:
			connID = item.Resource.ConnectionID
		case item.Binding != nil:
			resource, err := get(ctx, tx, policy.Scope, item.Binding.ResourceID)
			if err != nil {
				return err
			}
			connID = resource.Resource.ConnectionID
		case item.Domain != nil:
			connID = item.Domain.Request.ConnectionID
		case item.Backup != nil:
			resource, err := get(ctx, tx, policy.Scope, item.Backup.ResourceID)
			if err != nil {
				return err
			}
			connID = resource.Resource.ConnectionID
		}
		conn, err := get(ctx, tx, policy.Scope, connID)
		if err != nil {
			return err
		}
		if conn.Connection == nil || report.ConnectionGeneration != conn.Generation {
			return postgres.ErrStale
		}
		if (!conn.Connection.Enabled || conn.Action != "reconcile") && report.State == "ready" {
			return postgres.ErrStale
		}
		if report.State == "ready" {
			if policy.ValidateConnection(*conn.Connection) != nil || (item.Resource != nil && item.Resource.ConnectionGeneration != conn.Generation) || (item.Binding != nil && item.Binding.ConnectionGeneration != conn.Generation) || (item.Domain != nil && item.Domain.ConnectionGeneration != conn.Generation) {
				return postgres.ErrStale
			}
		}
	}
	if report.State == "disconnected" {
		if item.Action != "disconnect" && item.Action != "disconnected" {
			return postgres.ErrStale
		}
		item.Action = "disconnected"
		if item.Resource != nil {
			item.Resource.Owner = "external"
		}
	}
	item.Observation = &report
	delete(report.Details, "adoptionReviewHash")
	if item.Resource != nil && item.Resource.Mode == "connect" && report.State == "ready" {
		if grant, ok := policy.Adoptions[item.Resource.ExternalName]; ok {
			conn, err := get(ctx, tx, policy.Scope, item.Resource.ConnectionID)
			if err != nil {
				return err
			}
			if hash, err := cloud.AdoptionReview(*item.Resource, *conn.Connection, report.Details["inventoryHash"], grant); err == nil {
				report.Details["adoptionReviewHash"] = hash
			}
		}
	}
	item.State = report.State
	item.UpdatedAt = time.Now().UTC()
	if item.Resource != nil {
		item.Resource.State = report.State
	}
	if item.Binding != nil {
		item.Binding.State = report.State
	}
	if item.Domain != nil {
		item.Domain.OwnershipVerified = report.Details["ownershipVerified"] == "true"
		item.Domain.DNSReady = report.Details["dnsReady"] == "true"
		item.Domain.CertificateReady = report.Details["certificateReady"] == "true"
		item.Domain.State = report.State
		item.Domain.ObservedAt = &report.ObservedAt
	}
	if err := save(ctx, tx, policy, item); err != nil {
		return err
	}
	if err := refreshPlacements(ctx, tx, policy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
