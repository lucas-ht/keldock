package inspection

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Service reads the installation's configured projects through one shared policy boundary.
type Service struct {
	Store        *postgres.Store
	Organization string
	PublicURL    string
	Projects     map[string]v1.Project
}

func (service *Service) scope(principal access.Principal, project string) (v1.Scope, error) {
	configured, exists := service.Projects[project]
	if !exists || principal.User.OrganizationID != service.Organization || !principal.CanRead(project) {
		return v1.Scope{}, access.ErrDenied
	}
	return configured.Scope, nil
}

// ListProjects returns only projects allowed by both the account and the current credential.
func (service *Service) ListProjects(ctx context.Context, principal access.Principal) (Page[Project], error) {
	result := Page[Project]{Items: []Project{}}
	ids := []string{}
	for id := range service.Projects {
		if _, err := service.scope(principal, id); err == nil {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		item := Project{Project: service.Projects[id]}
		var policy []byte
		err := service.Store.Pool().QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE o.state='ready'),count(*) FILTER(WHERE o.state IN ('failed','canceled')),(SELECT preview_policy FROM projects WHERE organization_id=$1 AND id=$2) FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation WHERE e.organization_id=$1 AND e.project_id=$2 AND NOT e.deleted`, service.Organization, id).Scan(&item.Environments, &item.Ready, &item.Attention, &policy)
		if err != nil {
			return result, err
		}
		if err := json.Unmarshal(policy, &item.Previews); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// ListEnvironments paginates at most 25 complete rows for one explicitly authorized project.
func (service *Service) ListEnvironments(ctx context.Context, principal access.Principal, project, after string) (Page[Environment], error) {
	scope, err := service.scope(principal, project)
	if err != nil {
		return Page[Environment]{}, err
	}
	if len(after) > 128 {
		return Page[Environment]{}, access.ErrInvalid
	}
	return service.environments(ctx, scope, after, "")
}

// GetEnvironment never resolves an environment outside the requested project.
func (service *Service) GetEnvironment(ctx context.Context, principal access.Principal, project, id string) (Environment, error) {
	scope, err := service.scope(principal, project)
	if err != nil {
		return Environment{}, err
	}
	page, err := service.environments(ctx, scope, "", id)
	if err != nil {
		return Environment{}, err
	}
	if len(page.Items) != 1 {
		return Environment{}, postgres.ErrNotFound
	}
	return page.Items[0], nil
}
