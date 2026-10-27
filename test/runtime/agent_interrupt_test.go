//go:build integration

package runtime_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
)

type waitingDriver struct{ started, stopped chan struct{} }

func (driver waitingDriver) Reconcile(ctx context.Context, _ v1.ProjectedDeployment) error {
	close(driver.started)
	<-ctx.Done()
	close(driver.stopped)
	return ctx.Err()
}
func (waitingDriver) Observe(context.Context, v1.Placement, v1.Application, map[string]string, string) (v1.Observation, error) {
	return v1.Observation{}, nil
}
func (waitingDriver) Remove(context.Context, v1.Placement) (bool, error) { return false, nil }
func (waitingDriver) Suspend(context.Context, v1.Placement) error        { return nil }

func TestRevocationCancelsAnInFlightAgentDriver(t *testing.T) {
	fixture := newAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	fixture.submit(t, fixture.sign(t, applicationInput(t)), "revocation-in-flight", http.StatusAccepted)
	worker := controlplane.Worker{Store: fixture.store, Publisher: publisher(t), Organization: fixture.scope.OrganizationID}
	if worked, err := worker.Step(ctx); err != nil || !worked {
		t.Fatalf("publish: %v %v", worked, err)
	}
	client, err := agent.NewClient(fixture.server.URL, fixture.agentIdentity.Token, true)
	if err != nil {
		t.Fatal(err)
	}
	driver := waitingDriver{started: make(chan struct{}), stopped: make(chan struct{})}
	finished := make(chan error, 1)
	go func() { finished <- (agent.Agent{Client: client, Driver: driver}).Run(ctx) }()
	select {
	case <-driver.started:
	case <-ctx.Done():
		t.Fatal("driver did not start")
	}
	principal, _, err := fixture.api.Security.Users.Session(ctx, fixture.session)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.api.Security.Clusters.Revoke(ctx, principal); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		var rejected agent.APIError
		if !errors.As(err, &rejected) || rejected.Status != http.StatusForbidden {
			t.Fatalf("revoked agent did not surface rejection: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("revoked agent kept running")
	}
	select {
	case <-driver.stopped:
	default:
		t.Fatal("in-flight driver context was not canceled")
	}
}
