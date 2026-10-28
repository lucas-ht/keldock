package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Driver is the trusted cluster boundary; app workloads remain owned by Flux.
type Driver interface {
	Reconcile(context.Context, v1.ProjectedDeployment) error
	Observe(context.Context, v1.Placement, v1.Application, map[string]string, string) (v1.Observation, error)
	Remove(context.Context, v1.Placement) (bool, error)
	Suspend(context.Context, v1.Placement) error
}

// Agent serializes one cluster's approved projections through an outbound connection.
type Agent struct {
	Client *Client
	Driver Driver
}

// Step revalidates selections before mutation and reports outcomes without a CI session.
func (agent Agent) Step(ctx context.Context) error {
	items, err := agent.Client.Desired(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, listed := range items {
		current, err := agent.Client.Current(ctx, listed.Operation.EnvironmentID)
		if err != nil {
			var status APIError
			if errors.As(err, &status) && status.Status == http.StatusNotFound {
				continue
			}
			failures = append(failures, err)
			continue
		}
		if current.Operation.ID != listed.Operation.ID || current.Operation.Generation != listed.Operation.Generation || current.Operation.State != listed.Operation.State {
			continue
		}
		if err := agent.project(ctx, *current); err != nil {
			var status APIError
			if errors.As(err, &status) && (status.Status == http.StatusConflict || status.Status == http.StatusNotFound) {
				continue
			}
			failures = append(failures, fmt.Errorf("environment %s: %w", current.Operation.EnvironmentID, err))
		}
	}
	return errors.Join(failures...)
}

func (agent Agent) project(ctx context.Context, desired v1.ProjectedDeployment) error {
	if desired.Operation.State == "canceled" {
		return agent.Driver.Suspend(ctx, desired.Selection)
	}
	if desired.Operation.Kind == "delete" {
		done, err := agent.Driver.Remove(ctx, desired.Selection)
		if err != nil || !done {
			return err
		}
		return agent.Client.Observe(ctx, v1.Observation{EnvironmentID: desired.Selection.EnvironmentID, Generation: desired.Selection.Generation, State: "deleted", Reason: "Pruned", ObservedAt: time.Now().UTC()})
	}
	if desired.Operation.Kind != "deploy" || desired.Bundle == "" {
		return nil
	}
	if err := agent.Driver.Reconcile(ctx, desired); err != nil {
		return err
	}
	report, err := agent.Driver.Observe(ctx, desired.Selection, desired.Definition, desired.Images, desired.Bundle)
	if err != nil {
		return err
	}
	return agent.Client.Observe(ctx, report)
}

// Run serializes workload projections under the enrolled registrar lease.
func (agent Agent) Run(ctx context.Context) error {
	return (SessionLoop{Client: agent.Client, Step: agent.Step}).Run(ctx)
}
