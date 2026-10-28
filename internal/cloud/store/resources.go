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
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (store *Store) connection(ctx context.Context, tx pgx.Tx, policy cloud.Policy, item cloud.Intent, data json.RawMessage) (cloud.Intent, error) {
	var request cloud.ConnectionRequest
	if v1.DecodeJSON(data, &request) != nil {
		return item, access.ErrInvalid
	}
	current, err := get(ctx, tx, policy.Scope, item.ID)
	if err != nil && !errors.Is(err, postgres.ErrNotFound) {
		return item, err
	}
	if err == nil && (current.Connection == nil || request.Generation != current.Generation+1) {
		return item, postgres.ErrStale
	}
	if errors.Is(err, postgres.ErrNotFound) && request.Generation != 1 {
		return item, postgres.ErrStale
	}
	if errors.Is(err, postgres.ErrNotFound) {
		if err := count(ctx, tx, policy, "connection", "", 8); err != nil {
			return item, err
		}
	}
	item.Generation = request.Generation
	item.Connection = &cloud.Connection{CloudConnection: v1.CloudConnection{Scope: policy.Scope, ID: item.ID, Provider: request.Provider, Account: request.Account, Region: request.Region, Profiles: request.Profiles, IdentityRef: request.IdentityRef}, ClusterID: policy.ClusterID, Generation: item.Generation, Enabled: true}
	if policy.ValidateConnection(*item.Connection) != nil {
		return item, access.ErrDenied
	}
	return item, nil
}

func (store *Store) resource(ctx context.Context, tx pgx.Tx, policy cloud.Policy, item cloud.Intent, data json.RawMessage) (cloud.Intent, error) {
	var request cloud.ResourceRequest
	if v1.DecodeJSON(data, &request) != nil {
		return item, access.ErrInvalid
	}
	if _, err := get(ctx, tx, policy.Scope, item.ID); !errors.Is(err, postgres.ErrNotFound) {
		if err == nil {
			return item, postgres.ErrConflict
		}
		return item, err
	}
	conn, err := connection(ctx, tx, policy, request.ConnectionID)
	if err != nil {
		return item, err
	}
	if policy.ValidateResource(*conn.Connection, request) != nil {
		return item, access.ErrDenied
	}
	profile := policy.Profiles[request.Profile]
	if err := count(ctx, tx, policy, "resource", request.Profile, profile.MaxResources); err != nil {
		return item, err
	}
	external := request.ExistingName
	owner := "external"
	if request.Mode == "create" {
		external, err = policy.BucketName(*conn.Connection, item.ID, request.Profile)
		if err != nil {
			return item, err
		}
		owner = "crossplane"
	}
	item.Resource = &cloud.Resource{Resource: v1.Resource{Scope: policy.Scope, ID: item.ID, InstallationID: policy.ClusterID, Profile: request.Profile, Generation: 1, Retention: "retain"}, ConnectionID: conn.ID, ConnectionGeneration: conn.Generation, Kind: request.Kind, Mode: request.Mode, ExternalName: external, Owner: owner, State: "pending"}
	return item, nil
}

func readyResource(ctx context.Context, tx pgx.Tx, policy cloud.Policy, id string) (cloud.Intent, cloud.Intent, error) {
	item, err := get(ctx, tx, policy.Scope, id)
	if err != nil {
		return item, cloud.Intent{}, err
	}
	if item.Resource == nil || item.Action != "reconcile" || item.Observation == nil || item.Observation.Generation != item.Generation || item.Observation.State != "ready" {
		return item, cloud.Intent{}, postgres.ErrConflict
	}
	if time.Since(item.Observation.ObservedAt) > 2*time.Minute {
		return item, cloud.Intent{}, postgres.ErrStale
	}
	conn, err := connection(ctx, tx, policy, item.Resource.ConnectionID)
	if err == nil && item.Resource.ConnectionGeneration != conn.Generation {
		err = postgres.ErrStale
	}
	return item, conn, err
}
