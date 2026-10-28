package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
	"github.com/robfig/cron/v3"
)

func environment(ctx context.Context, tx pgx.Tx, policy cloud.Policy, id string, generation int64) (v1.Placement, v1.Application, error) {
	var placement v1.Placement
	var input v1.SubmitRelease
	var selection, data []byte
	err := tx.QueryRow(ctx, `SELECT d.selection,r.input FROM environments e JOIN deployments d ON d.environment_id=e.id AND d.generation=e.generation JOIN releases r ON r.id=d.release_id WHERE e.organization_id=$1 AND e.project_id=$2 AND e.cluster_id=$3 AND e.id=$4 AND e.generation=$5 AND NOT e.deleted AND (e.expires_at IS NULL OR e.expires_at>now()) FOR UPDATE OF e`, policy.OrganizationID, policy.ProjectID, policy.ClusterID, id, generation).Scan(&selection, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return placement, input.Definition, postgres.ErrStale
	}
	if err != nil {
		return placement, input.Definition, err
	}
	if err := json.Unmarshal(selection, &placement); err != nil {
		return placement, input.Definition, err
	}
	err = json.Unmarshal(data, &input)
	return placement, input.Definition, err
}

func (store *Store) binding(ctx context.Context, tx pgx.Tx, policy cloud.Policy, item cloud.Intent, data json.RawMessage) (cloud.Intent, error) {
	var request cloud.BindingRequest
	if v1.DecodeJSON(data, &request) != nil || cloud.ValidateBinding(request) != nil {
		return item, access.ErrInvalid
	}
	resource, conn, err := readyResource(ctx, tx, policy, request.ResourceID)
	if err != nil {
		return item, err
	}
	if resource.Resource.Kind != cloud.ObjectStorage || len(policy.EgressCIDRs) == 0 {
		return item, access.ErrDenied
	}
	placement, definition, err := environment(ctx, tx, policy, request.EnvironmentID, request.Generation)
	if err != nil {
		return item, err
	}
	profile := policy.Profiles[resource.Resource.Profile]
	binding := &cloud.Binding{ResourceBinding: v1.ResourceBinding{Scope: policy.Scope, ID: item.ID, ResourceID: resource.ID, EnvironmentID: request.EnvironmentID, Components: request.Components, Generation: 1}, ConnectionGeneration: conn.Generation, Access: request.Access, State: "pending"}
	for _, name := range request.Components {
		if _, ok := definition.Components[name]; !ok {
			return item, access.ErrDenied
		}
		sa, role := cloud.WorkloadIdentity(*conn.Connection, profile, request.EnvironmentID, name)
		binding.Consumers = append(binding.Consumers, cloud.ConsumerIdentity{Component: name, Namespace: placement.Namespace, ServiceAccount: sa, RoleARN: role, Prefix: "projects/" + policy.ProjectID + "/environments/" + request.EnvironmentID + "/components/" + name + "/"})
	}
	var duplicate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND kind='binding' AND intent->>'action' NOT IN ('disconnect','disconnected') AND intent->'binding'->>'environmentId'=$3 AND intent->'binding'->>'resourceId'=$4)`, policy.OrganizationID, policy.ProjectID, request.EnvironmentID, request.ResourceID).Scan(&duplicate); err != nil {
		return item, err
	}
	if duplicate {
		return item, postgres.ErrConflict
	}
	for _, consumer := range binding.Consumers {
		var conflict bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents i, jsonb_array_elements(i.intent->'binding'->'consumers') c WHERE i.organization_id=$1 AND i.project_id=$2 AND i.kind='binding' AND i.intent->>'action' NOT IN ('disconnect','disconnected') AND i.intent->'binding'->>'environmentId'=$3 AND c->>'component'=$4 AND c->>'roleArn'<>$5)`, policy.OrganizationID, policy.ProjectID, request.EnvironmentID, consumer.Component, consumer.RoleARN).Scan(&conflict); err != nil {
			return item, err
		}
		if conflict {
			return item, postgres.ErrConflict
		}
	}
	if err := count(ctx, tx, policy, "binding", "", 128); err != nil {
		return item, err
	}
	item.Binding = binding
	return item, nil
}

func (store *Store) domain(ctx context.Context, tx pgx.Tx, policy cloud.Policy, item cloud.Intent, data json.RawMessage) (cloud.Intent, error) {
	var request cloud.DomainRequest
	if v1.DecodeJSON(data, &request) != nil {
		return item, access.ErrInvalid
	}
	conn, err := connection(ctx, tx, policy, request.ConnectionID)
	if err != nil {
		return item, err
	}
	if policy.ValidateDomain(*conn.Connection, request) != nil {
		return item, access.ErrDenied
	}
	_, definition, err := environment(ctx, tx, policy, request.EnvironmentID, request.Generation)
	if err != nil {
		return item, err
	}
	component, ok := definition.Components[request.Component]
	if !ok || !component.Route {
		return item, access.ErrDenied
	}
	if err := count(ctx, tx, policy, "domain", request.Profile, policy.Profiles[request.Profile].MaxResources); err != nil {
		return item, err
	}
	item.Domain = &cloud.DomainClaim{Scope: policy.Scope, ID: item.ID, Generation: 1, ConnectionGeneration: conn.Generation, Request: request, State: "pending"}
	return item, nil
}

func (store *Store) backup(ctx context.Context, tx pgx.Tx, policy cloud.Policy, item cloud.Intent, data json.RawMessage) (cloud.Intent, error) {
	var request cloud.BackupRequest
	if v1.DecodeJSON(data, &request) != nil || !v1.ValidName(request.DatabaseID) || len(request.Schedule) > 64 {
		return item, access.ErrInvalid
	}
	if _, err := cron.ParseStandard(request.Schedule); err != nil || len(strings.Fields(request.Schedule)) != 5 {
		return item, access.ErrInvalid
	}
	target, ok := policy.Databases[request.DatabaseID]
	if !ok {
		return item, access.ErrDenied
	}
	resource, conn, err := readyResource(ctx, tx, policy, request.ResourceID)
	if err != nil {
		return item, err
	}
	if resource.Resource.Kind != cloud.BackupStore {
		return item, access.ErrDenied
	}
	var existing bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_intents WHERE organization_id=$1 AND project_id=$2 AND kind='backup' AND intent->'backup'->>'databaseId'=$3 AND intent->>'action' NOT IN ('disconnect','disconnected'))`, policy.OrganizationID, policy.ProjectID, request.DatabaseID).Scan(&existing); err != nil {
		return item, err
	}
	if existing {
		return item, postgres.ErrConflict
	}
	profile := policy.Profiles[resource.Resource.Profile]
	_, writer := cloud.WorkloadIdentity(*conn.Connection, profile, item.ID, "writer")
	_, reader := cloud.WorkloadIdentity(*conn.Connection, profile, item.ID, "restore")
	prefix := cloud.ArchivePrefix(item.Scope, item.ID, 1)
	item.Backup = &cloud.BackupBinding{ResourceID: resource.ID, DatabaseID: request.DatabaseID, Database: target, ArchiveGeneration: 1, ArchivePrefix: prefix, WriterRoleARN: writer, RestoreRoleARN: reader, Schedule: request.Schedule, Archives: []cloud.ArchiveGeneration{{Generation: 1, Prefix: prefix}}, RestoreName: "restore-" + cloud.BackupKey(item.Scope, item.ID), BaseAttempt: 1}
	return item, nil
}
