// Package operations is the shared nonproduction mutation boundary for HTTP and MCP.
package operations

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Delegation refreshes current source authority without accepting CI input from a client.
type Delegation interface {
	AuthorizeRelease(context.Context, v1.Source) error
}

// Service shares configuration, current identity and durable state across client adapters.
type Service struct {
	Store        *postgres.Store
	Users        *access.Repository
	MCP          *mcpidentity.Repository
	Projects     map[string]v1.Project
	Organization string
	GitHub       Delegation
}

func (service *Service) scope(principal access.Principal, project string) (v1.Scope, error) {
	configured, ok := service.Projects[project]
	if !ok || configured.OrganizationID != service.Organization || principal.User.OrganizationID != service.Organization || !principal.CanOperate(project) {
		return v1.Scope{}, access.ErrDenied
	}
	return configured.Scope, nil
}

func (service *Service) authority(principal access.Principal, project string) postgres.MutationAuthority {
	return func(ctx context.Context, tx pgx.Tx) error {
		if principal.Kind == "mcp" {
			return service.MCP.RequireOperator(ctx, tx, principal, project)
		}
		user, err := service.Users.RequireHuman(ctx, tx, principal)
		if err != nil {
			return err
		}
		if !(access.Principal{User: user, Kind: "session"}).CanOperate(project) {
			return access.ErrDenied
		}
		return nil
	}
}

func actor(principal access.Principal) string {
	if principal.Kind == "mcp" {
		return "mcp:user:" + principal.User.ID + ":grant:" + principal.GrantID
	}
	return "user:" + principal.User.ID
}

func validMutation(generation int64, key string, ids ...string) bool {
	if generation < 1 || len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, id := range ids {
		if len(id) < 4 || len(id) > 128 {
			return false
		}
	}
	return true
}

// Select only reuses a release that the trusted CI path has already admitted.
func (service *Service) Select(ctx context.Context, principal access.Principal, project string, input v1.ReleaseSelection) (v1.Receipt, error) {
	scope, err := service.scope(principal, project)
	if err != nil {
		return v1.Receipt{}, err
	}
	if !validMutation(input.Generation, input.RequestKey, input.EnvironmentID, input.ReleaseID) {
		return v1.Receipt{}, access.ErrInvalid
	}
	release, err := service.Store.ReleaseInput(ctx, scope, input.ReleaseID)
	if err != nil {
		return v1.Receipt{}, err
	}
	if service.GitHub != nil {
		if err = service.GitHub.AuthorizeRelease(ctx, release.Source); err != nil {
			return v1.Receipt{}, err
		}
	}
	return service.Store.SelectRelease(ctx, scope, actor(principal), input, service.authority(principal, project))
}

// Retry creates a new generation of the selected admitted release and preserves its inputs.
func (service *Service) Retry(ctx context.Context, principal access.Principal, project string, input v1.OperationMutation) (v1.Receipt, error) {
	scope, err := service.scope(principal, project)
	if err != nil {
		return v1.Receipt{}, err
	}
	if !validMutation(input.Generation, input.RequestKey, input.OperationID) {
		return v1.Receipt{}, access.ErrInvalid
	}
	if service.GitHub != nil {
		var releaseID string
		err = service.Store.Pool().QueryRow(ctx, `SELECT d.release_id FROM operations o JOIN deployments d ON d.id=o.deployment_id JOIN environments e ON e.id=o.environment_id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3`, input.OperationID, scope.OrganizationID, scope.ProjectID).Scan(&releaseID)
		if errors.Is(err, pgx.ErrNoRows) {
			return v1.Receipt{}, postgres.ErrNotFound
		}
		if err != nil {
			return v1.Receipt{}, err
		}
		release, err := service.Store.ReleaseInput(ctx, scope, releaseID)
		if err != nil {
			return v1.Receipt{}, err
		}
		if err = service.GitHub.AuthorizeRelease(ctx, release.Source); err != nil {
			return v1.Receipt{}, err
		}
	}
	return service.Store.Retry(ctx, scope, actor(principal), input.OperationID, input.Generation, input.RequestKey, service.authority(principal, project))
}

// Cancel stops future work for the exact selected revision without deleting retained effects.
func (service *Service) Cancel(ctx context.Context, principal access.Principal, project string, input v1.OperationMutation) (v1.Receipt, error) {
	scope, err := service.scope(principal, project)
	if err != nil {
		return v1.Receipt{}, err
	}
	if !validMutation(input.Generation, input.RequestKey, input.OperationID) {
		return v1.Receipt{}, access.ErrInvalid
	}
	return service.Store.CancelRevision(ctx, scope, actor(principal), input, service.authority(principal, project))
}
