package agent

import (
	"fmt"
	"os"
	"time"
)

// MarkHealth records progress of the process loop, including deliberate enrollment wait.
// It is not evidence of an enrolled/connected cluster; that state is stored by the API.
func MarkHealth(path string) error {
	if path == "" {
		return nil
	}
	if path != "/tmp/keldock-agent-health" {
		return fmt.Errorf("unsupported process health path")
	}
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600)
}

// CheckHealth rejects a stalled process whose loop/lease has made no recent progress.
func CheckHealth(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("agent process has not reported progress")
	}
	at, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil || time.Since(at) > 90*time.Second || at.After(time.Now().Add(time.Minute)) {
		return fmt.Errorf("agent process progress is stale")
	}
	return nil
}
