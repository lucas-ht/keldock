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
		if item.Resource == nil {
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
	return item, nil
}
