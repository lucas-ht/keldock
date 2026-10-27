package inspection

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

var sensitiveText = regexp.MustCompile(`(?i)(bearer\s+[^\s,;]+|(?:password|secret|token|authorization|api[_-]?key)\s*[=:]\s*[^\s,;]+|[a-z][a-z0-9+.-]*://[^\s/@]+:[^\s/@]+@|\b(?:kda_|kma_|kmr_|kds_|kdi_|kmc_|kin_)[a-zA-Z0-9_-]+)`)

func redactor(release v1.SubmitRelease) func(string) string {
	values := []string{}
	for _, component := range release.Definition.Components {
		for _, value := range component.Env {
			if len(value) >= 4 {
				values = append(values, value)
			}
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return func(value string) string {
		for _, secret := range values {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
		value = sensitiveText.ReplaceAllString(value, "[redacted]")
		value = strings.Map(func(r rune) rune {
			if r < 32 && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, value)
		runes := []rune(value)
		if len(runes) > 1024 {
			value = string(runes[:1024]) + "…"
		}
		return value
	}
}

func sanitize(observation *v1.Observation, clean func(string) string) {
	observation.Reason = clean(observation.Reason)
	for index := range observation.Components {
		observation.Components[index].Message = clean(observation.Components[index].Message)
		observation.Components[index].Reason = clean(observation.Components[index].Reason)
	}
}

// GetOperation exposes the same bounded revision evidence as the CI receipt endpoint.
func (service *Service) GetOperation(ctx context.Context, principal access.Principal, project, id string) (Status, error) {
	scope, err := service.scope(principal, project)
	if err != nil {
		return Status{}, err
	}
	operation, err := service.Store.Operation(ctx, scope, id)
	if err != nil {
		return Status{}, err
	}
	observation, err := service.Store.LatestObservation(ctx, scope, id)
	if err != nil {
		return Status{}, err
	}
	var input []byte
	err = service.Store.Pool().QueryRow(ctx, `SELECT coalesce(r.input,'{}'::jsonb) FROM operations o JOIN environments e ON e.id=o.environment_id LEFT JOIN deployments d ON d.id=o.deployment_id LEFT JOIN releases r ON r.id=d.release_id WHERE o.id=$1 AND e.organization_id=$2 AND e.project_id=$3`, id, scope.OrganizationID, scope.ProjectID).Scan(&input)
	if err != nil {
		return Status{}, err
	}
	var release v1.SubmitRelease
	if err := json.Unmarshal(input, &release); err != nil {
		return Status{}, err
	}
	clean := redactor(release)
	operation.Error = clean(operation.Error)
	result := Status{APIVersion: v1.APIVersion, Operation: operation, Observation: observation, Stale: true}
	if observation != nil {
		sanitize(observation, clean)
		result.ObservationAgeSeconds = max(0, time.Since(observation.ObservedAt).Seconds())
		result.Stale = result.ObservationAgeSeconds > 30
	}
	return result, nil
}

// GetDiagnostics reads approved controller evidence and never fetches arbitrary logs.
func (service *Service) GetDiagnostics(ctx context.Context, principal access.Principal, project, id string) (Diagnostics, error) {
	item, err := service.GetEnvironment(ctx, principal, project, id)
	if err != nil {
		return Diagnostics{}, err
	}
	result := Diagnostics{EnvironmentID: item.ID, Generation: item.Generation, State: item.Operation.State, Stale: item.Stale, ObservationAgeSeconds: item.ObservationAgeSeconds, PublicationError: item.Operation.Error, Components: []v1.ComponentHealth{}}
	if item.Observation != nil {
		result.Components = item.Observation.Components
	}
	return result, nil
}

// Timeline returns the last 50 durable selection events for this environment.
func (service *Service) Timeline(ctx context.Context, principal access.Principal, project, id string) ([]Event, error) {
	if _, err := service.GetEnvironment(ctx, principal, project, id); err != nil {
		return nil, err
	}
	rows, err := service.Store.Pool().Query(ctx, `SELECT id,actor,event,generation,created_at FROM audit_events WHERE organization_id=$1 AND project_id=$2 AND target=$3 ORDER BY id DESC LIMIT 50`, service.Organization, project, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Event{}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Actor, &event.Event, &event.Generation, &event.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}
