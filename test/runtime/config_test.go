//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

type localConfig struct {
	DatabaseURL  string `json:"databaseUrl"`
	Kubeconfig   string `json:"kubeconfig"`
	Context      string `json:"context"`
	RegistryPush string `json:"registryPush"`
	RegistryPull string `json:"registryPull"`
}

func configuration(t *testing.T) localConfig {
	t.Helper()
	path, err := filepath.Abs("../../.local/runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("run task local:up first: %v", err)
	}
	var config localConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(config.DatabaseURL)
	if err != nil || parsed.Hostname() != "127.0.0.1" || parsed.Port() != "55432" || parsed.Path != "/keldock" || config.Context != "k3d-keldock-m1" || config.RegistryPush != "localhost:5010/keldock/bundles" {
		t.Fatal("refusing integration tests outside the dedicated local target")
	}
	return config
}

func database(t *testing.T) (*postgres.Store, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	store, err := postgres.Open(ctx, configuration(t).DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store, ctx
}
