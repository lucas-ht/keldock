package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
)

func TestRevokedPackagedAgentWaitsForOperator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := os.WriteFile(path, []byte("old synthetic identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	called := make(chan struct{}, 2)
	result := make(chan error, 1)
	stop := errors.New("replacement reached")
	go func() {
		calls := 0
		result <- runEnrolled(ctx, path, "", true, func() error {
			calls++
			called <- struct{}{}
			if calls == 1 {
				return agent.APIError{Status: 403}
			}
			return stop
		})
	}()
	<-called
	select {
	case <-called:
		t.Fatal("rejected credential retried without operator replacement")
	case <-time.After(50 * time.Millisecond):
	}
	if err := os.WriteFile(path, []byte("replacement synthetic identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, stop) {
		t.Fatal("replacement was not loaded", err)
	}
}
