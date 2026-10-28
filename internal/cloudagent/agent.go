// Package cloudagent projects optional approved cloud intent, separately from workload registration.
package cloudagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/cloud"
)

// Agent uses an independent cloud credential and locally configured provisioning policy.
type Agent struct {
	Client   *agent.Client
	Driver   *Driver
	Policies map[string]cloud.Policy
}

// Run shares the hardened outbound lease/rotation lifecycle with the registrar.
func (worker *Agent) Run(ctx context.Context) error {
	return (agent.SessionLoop{Client: worker.Client, Step: worker.Step}).Run(ctx)
}

// Step verifies local policy and re-fetches each revision immediately before cluster mutation.
func (worker *Agent) Step(ctx context.Context) error {
	var snapshot struct {
		Items []cloud.Intent `json:"items"`
	}
	if err := worker.Client.CloudRPC(ctx, http.MethodGet, "desired", nil, &snapshot); err != nil {
		return err
	}
	if _, err := worker.validateSnapshot(snapshot.Items); err != nil {
		return err
	}
	slices.SortStableFunc(snapshot.Items, func(a, b cloud.Intent) int { return order(a.Kind) - order(b.Kind) })
	var failures []error
	for _, listed := range snapshot.Items {
		// Shared IAM policies depend on every grant and the current parent connection.
		// Refresh the complete snapshot so a revoked sibling cannot be restored by
		// reconciling another binding from the beginning of this polling cycle.
		var fresh struct {
			Items []cloud.Intent `json:"items"`
		}
		if err := worker.Client.CloudRPC(ctx, http.MethodGet, "desired", nil, &fresh); err != nil {
			return err
		}
		groups, err := worker.validateSnapshot(fresh.Items)
		if err != nil {
			return err
		}
		items := groups[listed.ProjectID]
		current, exists := items[listed.ID]
		if !exists || current.Generation != listed.Generation || current.Action != listed.Action {
			continue
		}
		policy := worker.Policies[current.ProjectID]
		report := worker.Driver.Reconcile(ctx, policy, current, items)
		report.ProjectID = current.ProjectID
		report.ID = current.ID
		report.Generation = current.Generation
		report.ObservedAt = time.Now().UTC()
		if err := worker.Client.CloudRPC(ctx, http.MethodPost, "observations", report, nil); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (worker *Agent) validateSnapshot(snapshot []cloud.Intent) (map[string]map[string]cloud.Intent, error) {
	if len(snapshot) > 1000 {
		return nil, fmt.Errorf("cloud inventory exceeds supported bound")
	}
	items := map[string]map[string]cloud.Intent{}
	counts := map[string]int{}
	for _, item := range snapshot {
		policy, ok := worker.Policies[item.ProjectID]
		if !ok || policy.Scope != item.Scope || policy.Validate() != nil {
			return nil, fmt.Errorf("cloud snapshot outside local operator policy")
		}
		if item.Resource != nil && item.Action != "disconnected" {
			key := item.ProjectID + "/" + item.Resource.Profile
			counts[key]++
			if counts[key] > policy.Profiles[item.Resource.Profile].MaxResources {
				return nil, fmt.Errorf("cloud snapshot exceeds local resource capacity")
			}
		}
		if items[item.ProjectID] == nil {
			items[item.ProjectID] = map[string]cloud.Intent{}
		}
		if _, duplicate := items[item.ProjectID][item.ID]; duplicate {
			return nil, fmt.Errorf("duplicate cloud inventory identity")
		}
		items[item.ProjectID][item.ID] = item
	}
	return items, nil
}

func order(kind string) int {
	switch kind {
	case "connection":
		return 0
	case "resource":
		return 1
	case "binding":
		return 2
	case "domain":
		return 3
	case "backup":
		return 4
	}
	return 5
}
