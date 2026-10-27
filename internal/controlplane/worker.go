// Package controlplane executes durable admitted work independently of submitting clients.
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lucas-ht/keldock/internal/compiler"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// BundlePublisher writes content-addressed deployment artifacts; it never builds app images.
type BundlePublisher interface {
	Push(context.Context, compiler.Bundle) (string, error)
}

// Worker owns one bounded dispatch loop. Multiple instances coordinate through database leases.
type Worker struct {
	Store        *postgres.Store
	Publisher    BundlePublisher
	Organization string
	Lease        time.Duration
	Timeout      time.Duration
}

// Step handles one operation. A canceled worker leaves its lease for durable recovery.
func (worker Worker) Step(ctx context.Context) (bool, error) {
	lease, timeout := worker.Lease, worker.Timeout
	if lease == 0 {
		lease = time.Minute
	}
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	if timeout <= 0 || timeout >= lease {
		return false, fmt.Errorf("operation timeout must be shorter than its lease")
	}
	work, err := worker.Store.Claim(ctx, worker.Organization, lease)
	if err != nil || work == nil {
		return false, err
	}
	operation, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	bundle, err := compiler.Render(work.Input, work.Selection)
	var reference string
	if err == nil {
		reference, err = worker.Publisher.Push(operation, bundle)
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if err != nil {
		if recordErr := worker.Store.Failed(ctx, *work, err.Error()); recordErr != nil && !errors.Is(recordErr, postgres.ErrStale) {
			return true, fmt.Errorf("record publication failure: %w", recordErr)
		}
		return true, fmt.Errorf("publication operation %s: %w", work.Operation.ID, err)
	}
	if err := worker.Store.Published(ctx, *work, reference); err != nil && !errors.Is(err, postgres.ErrStale) {
		return true, err
	}
	return true, nil
}

// Run polls pending work and preview expiry until shutdown; status is kept in PostgreSQL.
func (worker Worker) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := worker.Store.ExpirePreviews(ctx, worker.Organization); err != nil {
			slog.Warn("preview expiry retry", "error", err)
		}
		didWork, err := worker.Step(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Warn("worker retry", "error", err)
		}
		if didWork && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}
