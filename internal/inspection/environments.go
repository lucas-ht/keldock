package inspection

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func (service *Service) environments(ctx context.Context, scope v1.Scope, after, id string) (Page[Environment], error) {
	result := Page[Environment]{Items: []Environment{}}
	rows, err := service.Store.Pool().Query(ctx, `SELECT e.id,e.project_id,e.application,e.cluster_id,e.pull_request,e.generation,e.deleted,e.expires_at,
 o.id,coalesce(o.deployment_id,''),o.kind,o.state,o.attempt,o.error,o.updated_at,
 coalesce(d.release_id,''),coalesce(d.selection->>'hostname',''),coalesce(r.input,'{}'::jsonb),v.report
 FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation
 LEFT JOIN deployments d ON d.id=o.deployment_id LEFT JOIN releases r ON r.id=d.release_id
 LEFT JOIN observations v ON v.environment_id=e.id AND v.generation=e.generation
 WHERE e.organization_id=$1 AND e.project_id=$2 AND e.id>$3 AND ($4='' OR e.id=$4) ORDER BY e.id LIMIT 26`, scope.OrganizationID, scope.ProjectID, after, id)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Environment
		var input, report []byte
		err := rows.Scan(&item.ID, &item.ProjectID, &item.Application, &item.ClusterID, &item.PullRequest, &item.Generation, &item.Deleted, &item.ExpiresAt,
			&item.Operation.ID, &item.Operation.DeploymentID, &item.Operation.Kind, &item.Operation.State, &item.Operation.Attempt, &item.Operation.Error, &item.Operation.UpdatedAt,
			&item.ReleaseID, &item.Hostname, &input, &report)
		if err != nil {
			return result, err
		}
		item.Operation.Scope = scope
		item.Operation.EnvironmentID = item.ID
		item.Operation.Generation = item.Generation
		var release v1.SubmitRelease
		if err := json.Unmarshal(input, &release); err != nil {
			return result, err
		}
		item.Images = release.Images
		if item.Images == nil {
			item.Images = map[string]string{}
		}
		item.Components = []Component{}
		names := make([]string, 0, len(release.Definition.Components))
		for name := range release.Definition.Components {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			component := release.Definition.Components[name]
			item.Components = append(item.Components, Component{Name: name, Kind: component.Kind, Image: item.Images[name], DependsOn: component.DependsOn})
		}
		if item.ReleaseID != "" {
			item.Source = &release.Source
		}
		clean := redactor(release)
		item.Operation.Error = clean(item.Operation.Error)
		item.Stale = true
		if len(report) > 0 {
			var observation v1.Observation
			if err := json.Unmarshal(report, &observation); err != nil {
				return result, err
			}
			sanitize(&observation, clean)
			item.Observation = &observation
			item.ObservationAgeSeconds = max(0, time.Since(observation.ObservedAt).Seconds())
			item.Stale = item.ObservationAgeSeconds > 30
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > 25 {
		result.Items = result.Items[:25]
		result.NextCursor = result.Items[24].ID
	}
	return result, nil
}
