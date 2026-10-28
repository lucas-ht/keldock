package agent

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// SessionLoop maintains one exclusive outbound lease and cancels work when it is lost.
// Workload and cloud protocols share transport mechanics, never credential authority.
type SessionLoop struct {
	Client *Client
	Step   func(context.Context) error
}

// Run retains workloads on disconnection and cancels in-flight work when the lease is lost.
func (loop SessionLoop) Run(ctx context.Context) error {
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := loop.Client.Release(release); err != nil {
			slog.Debug("agent lease release unavailable", "error", err)
		}
	}()
	for ctx.Err() == nil {
		err := loop.connected(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var status APIError
		if errors.As(err, &status) && (status.Status == http.StatusUnauthorized || status.Status == http.StatusForbidden) {
			return err
		}
		slog.Warn("agent connection/reconciliation retry", "error", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

func (loop SessionLoop) connected(parent context.Context) (result error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if err := loop.Client.RefreshCredential(ctx); err != nil {
		return err
	}
	if err := loop.Client.Heartbeat(ctx); err != nil {
		return err
	}
	heartbeat := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				heartbeat <- ctx.Err()
				return
			case <-ticker.C:
				if err := loop.Client.Heartbeat(ctx); err != nil {
					heartbeat <- err
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		cause := <-heartbeat
		if errors.Is(result, context.Canceled) && parent.Err() == nil {
			result = cause
		}
	}()
	for {
		err := loop.Client.RefreshCredential(ctx)
		if err == nil {
			err = loop.Step(ctx)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			var status APIError
			if errors.As(err, &status) && (status.Status == http.StatusUnauthorized || status.Status == http.StatusForbidden) {
				return err
			}
			slog.Warn("agent observation/reconciliation retry", "error", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
}
