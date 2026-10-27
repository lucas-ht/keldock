//go:build integration

package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/compiler"
	"github.com/lucas-ht/keldock/internal/controlplane"
)

type interruptedPublisher struct {
	publisher *artifacts.Publisher
	cancel    context.CancelFunc
	reference string
}

func (publisher *interruptedPublisher) Push(ctx context.Context, bundle compiler.Bundle) (string, error) {
	reference, err := publisher.publisher.Push(ctx, bundle)
	if err != nil {
		return "", err
	}
	publisher.reference = reference
	publisher.cancel()
	return "", context.Canceled
}

func TestWorkerRecoversPublishedPartialEffect(t *testing.T) {
	store, ctx := database(t)
	scope, input := scopedInput(t, store, ctx)
	receipt, err := store.Admit(ctx, scope, "ci", "partial-effect-request", "sample.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	interrupted, cancel := context.WithCancel(ctx)
	defer cancel()
	publisher := &interruptedPublisher{publisher: publisher(t), cancel: cancel}
	worker := controlplane.Worker{Store: store, Publisher: publisher, Organization: scope.OrganizationID, Lease: 2 * time.Second, Timeout: time.Second}
	if didWork, err := worker.Step(interrupted); !didWork || !errors.Is(err, context.Canceled) || publisher.reference == "" {
		t.Fatalf("interruption before durable completion: %v %v", didWork, err)
	}
	state, err := store.Operation(ctx, scope, receipt.OperationID)
	if err != nil || state.State != "publishing" {
		t.Fatalf("partial work was not recoverable: %+v %v", state, err)
	}
	time.Sleep(2100 * time.Millisecond)
	restarted := controlplane.Worker{Store: store, Publisher: publisher.publisher, Organization: scope.OrganizationID}
	if didWork, err := restarted.Step(ctx); !didWork || err != nil {
		t.Fatalf("restart: %v %v", didWork, err)
	}
	desired, err := store.Desired(ctx, scope.OrganizationID, "keldock-m1")
	if err != nil || len(desired) != 1 || desired[0].Bundle != publisher.reference || desired[0].Operation.State != "reconciling" {
		t.Fatalf("recovery changed immutable bundle or lost dispatch: %+v %v", desired, err)
	}
}
