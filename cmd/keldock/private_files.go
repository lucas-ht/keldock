package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
)

func privateFilesCommand(args []string) error {
	flags := flag.NewFlagSet("init-files", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	source := flags.String("source", "/input", "Kubernetes Secret projection")
	destination := flags.String("destination", "/runtime/private", "private process configuration")
	state := flags.String("state", "/state/private", "private persistent credential directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	for _, path := range []string{*destination, *state} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	for _, name := range []string{"control.json", "agent.json", "database-agent.json", "webhook.secret", "postgres-password", "registry-password", "registry-pulls.json"} {
		path := filepath.Join(*source, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
			return fmt.Errorf("invalid projected configuration file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*destination, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func waitForEnrollment(ctx context.Context, path, healthFile string) error {
	slog.Info("waiting for explicit cluster enrollment", "credentialFile", path)
	for {
		if err := agent.MarkHealth(healthFile); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
				return fmt.Errorf("invalid enrollment file permissions")
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func processHealthCommand(args []string) error {
	flags := flag.NewFlagSet("process-health", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("file", "/tmp/keldock-agent-health", "process progress file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return agent.CheckHealth(*path)
}
