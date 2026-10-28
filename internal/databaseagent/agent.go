package databaseagent

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/database"
)

// Agent serializes database effects under an independent server lease and local policy.
type Agent struct {
	Client     *agent.Client
	Driver     *Driver
	Policies   map[string]database.Policy
	HealthFile string
}

// Step revalidates every allocation immediately before applying its effects.
func (a Agent) Step(ctx context.Context) error {
	var page struct {
		Items []database.Allocation `json:"items"`
	}
	if err := a.Client.DatabaseRPC(ctx, "GET", "desired", nil, &page); err != nil {
		return err
	}
	if len(page.Items) > 1000 {
		return fmt.Errorf("database inventory limit exceeded")
	}
	seen := map[string]bool{}
	for _, item := range page.Items {
		key := item.OrganizationID + "/" + item.ProjectID + "/" + item.ID
		if seen[key] {
			return fmt.Errorf("duplicate database allocation")
		}
		seen[key] = true
		p, ok := a.Policies[item.ProjectID]
		if !ok || !item.ValidProjection(p) {
			return fmt.Errorf("database projection outside operator delegation")
		}
	}
	for _, item := range page.Items {
		var current database.Allocation
		if err := a.Client.DatabaseRPC(ctx, "GET", "allocations/"+item.ID+"?project="+url.QueryEscape(item.ProjectID), nil, &current); err != nil {
			return err
		}
		if current.Generation != item.Generation || !current.ValidProjection(a.Policies[item.ProjectID]) {
			continue
		}
		e := a.Driver.Reconcile(ctx, a.Policies[item.ProjectID], current)
		e.ObservedAt = time.Now().UTC()
		if err := a.Client.DatabaseRPC(ctx, "POST", "observations", e, nil); err != nil {
			return err
		}
	}
	return nil
}

// Run uses the same bounded transport loop while keeping all authority separate.
func (a Agent) Run(ctx context.Context) error {
	return (agent.SessionLoop{Client: a.Client, Step: a.Step, HealthFile: a.HealthFile}).Run(ctx)
}
