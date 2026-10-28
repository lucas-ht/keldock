package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (store *Store) lifecycle(ctx context.Context, tx pgx.Tx, policy cloud.Policy, principal access.Principal, id string, data json.RawMessage) (cloud.Intent, error) {
	item, err := get(ctx, tx, policy.Scope, id)
	if err != nil {
		return item, err
	}
	var request cloud.Mutation
	if v1.DecodeJSON(data, &request) != nil {
		return item, access.ErrInvalid
	}
	if request.Generation != item.Generation {
		return item, postgres.ErrStale
	}
	switch request.Action {
	case "pause-schedule", "resume-schedule":
		if item.Backup == nil || item.Action != "reconcile" {
			return item, access.ErrInvalid
		}
		item.Backup.SchedulePaused = request.Action == "pause-schedule"
		request.Action = "reconcile"
	case "adopt":
		if !principal.IsAdmin() || item.Resource == nil || item.Resource.Mode != "connect" || item.Resource.Owner != "external" || item.Action != "reconcile" || item.Observation == nil || item.Observation.State != "ready" || time.Since(item.Observation.ObservedAt) > 2*time.Minute {
			return item, access.ErrDenied
		}
		conn, err := connection(ctx, tx, policy, item.Resource.ConnectionID)
		if err != nil {
			return item, err
		}
		grant, ok := policy.Adoptions[item.Resource.ExternalName]
		if !ok {
			return item, access.ErrDenied
		}
		review, err := cloud.AdoptionReview(*item.Resource, *conn.Connection, item.Observation.Details["inventoryHash"], grant)
		if err != nil || review != request.ReviewHash {
			return item, postgres.ErrStale
		}
		item.Resource.AdoptionGeneration = item.Generation
		item.Resource.AdoptionReviewHash = review
		item.Resource.Mode = "adopt"
		item.Resource.Owner = "crossplane"
		request.Action = "reconcile"
	case "retry":
		if item.Connection != nil || item.Action != "reconcile" {
			return item, access.ErrInvalid
		}
		if item.Backup != nil {
			if item.Backup.BaseAttempt < 1 {
				item.Backup.BaseAttempt = 1
			}
			if item.Backup.BaseAttempt >= 10 {
				return item, access.ErrInvalid
			}
			item.Backup.BaseAttempt++
		}
		request.Action = "reconcile"
	case "next-archive":
		if item.Backup == nil || item.Action != "reconcile" || len(item.Backup.Archives) >= 16 {
			return item, access.ErrInvalid
		}
		item.Backup.ArchiveGeneration++
		item.Backup.BaseAttempt = 1
		item.Backup.ArchivePrefix = cloud.ArchivePrefix(item.Scope, item.ID, item.Backup.ArchiveGeneration)
		item.Backup.Archives = append(item.Backup.Archives, cloud.ArchiveGeneration{Generation: item.Backup.ArchiveGeneration, Prefix: item.Backup.ArchivePrefix})
		request.Action = "reconcile"
	case "restore":
		if item.Backup == nil || item.Action != "reconcile" || item.Backup.RestoreGeneration != 0 {
			return item, access.ErrInvalid
		}
		found := false
		for _, archive := range item.Backup.Archives {
			if archive.Generation == request.ArchiveGeneration {
				found = true
			}
		}
		if !found {
			return item, access.ErrInvalid
		}
		item.Backup.RestoreGeneration = request.ArchiveGeneration
		request.Action = "reconcile"
	case "suspend":
		if item.Connection == nil || !principal.IsAdmin() {
			return item, access.ErrDenied
		}
		item.Connection.Enabled = false
	case "retain", "cancel", "disconnect":
		if item.Connection != nil {
			return item, access.ErrInvalid
		}
		if item.Resource != nil {
			var bound bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND (intent->'binding'->>'resourceId'=$3 OR intent->'backup'->>'resourceId'=$3) AND intent->>'action' NOT IN ('disconnect','disconnected'))`, policy.OrganizationID, policy.ProjectID, id).Scan(&bound); err != nil {
				return item, err
			}
			if bound {
				return item, postgres.ErrConflict
			}
		}
		if request.Action == "cancel" {
			request.Action = "retain"
		}
	case "refresh":
		if item.Resource == nil || item.Action == "disconnected" {
			return item, access.ErrInvalid
		}
		conn, err := connection(ctx, tx, policy, item.Resource.ConnectionID)
		if err != nil {
			return item, err
		}
		item.Resource.ConnectionGeneration = conn.Generation
		request.Action = "reconcile"
	default:
		return item, access.ErrInvalid
	}
	item.Generation++
	item.Action = request.Action
	item.State = "pending"
	item.Observation = nil
	item.UpdatedAt = time.Now().UTC()
	if item.Connection != nil {
		item.Connection.Generation = item.Generation
	}
	if item.Resource != nil {
		item.Resource.Generation = item.Generation
		item.Resource.State = "pending"
	}
	if item.Binding != nil {
		item.Binding.Generation = item.Generation
		item.Binding.State = "pending"
	}
	if item.Domain != nil {
		item.Domain.Generation = item.Generation
		item.Domain.State = "pending"
	}
	return item, nil
}
