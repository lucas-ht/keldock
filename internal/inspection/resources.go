package inspection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/database"
)

// Resource exposes lifecycle and recovery evidence without identity/credential references.
type Resource struct {
	ID            string                   `json:"id"`
	Kind          string                   `json:"kind"`
	EnvironmentID string                   `json:"environmentId"`
	Alias         string                   `json:"alias"`
	Profile       string                   `json:"profile"`
	Generation    int64                    `json:"generation"`
	State         string                   `json:"state"`
	Reason        string                   `json:"reason,omitempty"`
	Retained      bool                     `json:"retained"`
	Stale         bool                     `json:"stale"`
	Backup        *database.BackupEvidence `json:"backup,omitempty"`
}

// ListResources shares current account/project authority with the console and MCP.
func (s *Service) ListResources(ctx context.Context, p access.Principal, project, after string) (Page[Resource], error) {
	page := Page[Resource]{Items: []Resource{}}
	scope, err := s.scope(p, project)
	if err != nil {
		return page, err
	}
	if len(after) > 128 {
		return page, access.ErrInvalid
	}
	rows, err := s.Store.Pool().Query(ctx, `SELECT intent,observation FROM database_allocations WHERE organization_id=$1 AND project_id=$2 AND id>$3 ORDER BY id LIMIT 26`, scope.OrganizationID, scope.ProjectID, after)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var intent, observation []byte
		if err := rows.Scan(&intent, &observation); err != nil {
			return page, err
		}
		var a database.Allocation
		if err := json.Unmarshal(intent, &a); err != nil {
			return page, err
		}
		r := Resource{ID: a.ID, Kind: "postgresql", EnvironmentID: a.EnvironmentID, Alias: a.Alias, Profile: a.Profile, Generation: a.Generation, State: "pending", Retained: a.Action == "retain"}
		if len(observation) > 0 {
			var e database.Evidence
			if err := json.Unmarshal(observation, &e); err != nil {
				return page, err
			}
			if e.Generation == a.Generation {
				r.State = e.State
				r.Reason = e.Reason
				r.Stale = time.Since(e.ObservedAt) > 90*time.Second
				r.Backup = e.Backup
			}
		}
		page.Items = append(page.Items, r)
	}
	if len(page.Items) > 25 {
		page.Items = page.Items[:25]
		page.NextCursor = page.Items[24].ID
	}
	return page, rows.Err()
}
