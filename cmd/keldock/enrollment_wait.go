package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
)

// runEnrolled keeps explicitly waiting packaged agents available for operator
// enrollment after revocation. It never retries the rejected credential or
// performs reconciliation until an operator changes the private identity file.
func runEnrolled(ctx context.Context, path, health string, wait bool, run func() error) error {
	if !wait {
		return run()
	}
	for ctx.Err() == nil {
		if err := waitForEnrollment(ctx, path, health); err != nil {
			return err
		}
		before, err := enrollmentFingerprint(path)
		if err != nil {
			return err
		}
		err = run()
		var status agent.APIError
		if !errors.As(err, &status) || (status.Status != 401 && status.Status != 403) {
			return err
		}
		slog.Warn("agent credential rejected; waiting for explicit operator replacement")
		if err := waitEnrollmentReplacement(ctx, path, health, before); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func enrollmentFingerprint(path string) ([32]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return [32]byte{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 8192 {
		return [32]byte{}, fmt.Errorf("invalid private enrollment file")
	}
	data, err := os.ReadFile(path)
	return sha256.Sum256(data), err
}

func waitEnrollmentReplacement(ctx context.Context, path, health string, before [32]byte) error {
	for {
		if err := agent.MarkHealth(health); err != nil {
			return err
		}
		current, err := enrollmentFingerprint(path)
		if err == nil && current != before {
			return nil
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
