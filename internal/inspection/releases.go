package inspection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Release exposes reusable admitted identity and digests, excluding runtime values and signatures.
type Release struct {
	ID          string            `json:"id"`
	Application string            `json:"application"`
	Source      v1.Source         `json:"source"`
	Images      map[string]string `json:"images"`
	AdmittedAt  time.Time         `json:"admittedAt"`
}

// ListReleases returns a bounded, project-scoped page of CI-admitted immutable releases.
func (service *Service) ListReleases(ctx context.Context, principal access.Principal, project, after string) (Page[Release], error) {
	result := Page[Release]{Items: []Release{}}
	scope, err := service.scope(principal, project)
	if err != nil {
		return result, err
	}
	if len(after) > 128 {
		return result, access.ErrInvalid
	}
	rows, err := service.Store.Pool().Query(ctx, `SELECT id,input,admitted_at FROM releases WHERE organization_id=$1 AND project_id=$2 AND id>$3 ORDER BY id LIMIT 26`, scope.OrganizationID, scope.ProjectID, after)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Release
		var data []byte
		if err = rows.Scan(&item.ID, &data, &item.AdmittedAt); err != nil {
			return result, err
		}
		var input v1.SubmitRelease
		if err = json.Unmarshal(data, &input); err != nil {
			return result, err
		}
		item.Application = input.Definition.Name
		item.Source = input.Source
		item.Images = input.Images
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > 25 {
		result.Items = result.Items[:25]
		result.NextCursor = result.Items[24].ID
	}
	return result, nil
}
