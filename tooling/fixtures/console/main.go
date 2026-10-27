// This development harness seeds synthetic data only in the disposable keldock-m1 target.
// It is not part of the installed control plane or a public bootstrap path.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/cluster"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/lucas-ht/keldock/internal/controlplane/authentication"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
	"golang.org/x/sync/errgroup"
)

type configuration struct {
	DatabaseURL  string `json:"databaseUrl"`
	Kubeconfig   string `json:"kubeconfig"`
	Context      string `json:"context"`
	RegistryPush string `json:"registryPush"`
	RegistryPull string `json:"registryPull"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("local console fixture stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	data, err := os.ReadFile(".local/runtime.json")
	if err != nil {
		return err
	}
	var config configuration
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	database, err := url.Parse(config.DatabaseURL)
	if err != nil || database.Host != "127.0.0.1:55432" || database.Path != "/keldock" || config.Context != "k3d-keldock-m1" || config.RegistryPush != "localhost:5010/keldock/bundles" {
		return fmt.Errorf("refusing console fixture outside disposable keldock-m1")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:3801")
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	store, err := postgres.Open(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	projects := map[string]v1.Project{}
	for index, id := range []string{"sample", "empty-project"} {
		project := v1.Project{Scope: v1.Scope{OrganizationID: "console-local", ProjectID: id}, Name: map[bool]string{true: "Sample application", false: "Empty project"}[index == 0], RepositoryID: int64(42 + index), ClusterID: "keldock-m1"}
		if err := store.SeedProject(ctx, project); err != nil {
			return err
		}
		projects[id] = project
	}
	users, err := access.NewRepository(store.Pool(), "console-local")
	if err != nil {
		return err
	}
	principal, err := accounts(ctx, users, store)
	if err != nil {
		return err
	}
	clusters, err := clusteridentity.NewRepository(store.Pool(), users, "keldock-m1")
	if err != nil {
		return err
	}
	if err := clusters.Seed(ctx, "Disposable development cluster"); err != nil {
		return err
	}
	mcp, err := mcpidentity.NewRepository(store.Pool(), users, "http://127.0.0.1:3800")
	if err != nil {
		return err
	}
	if err := mcp.Seed(ctx); err != nil {
		return err
	}
	security, err := authentication.New(users, clusters, mcp, authentication.Config{PublicURL: "http://127.0.0.1:3800", Development: true, ClusterID: "keldock-m1", Projects: []string{"sample", "empty-project"}})
	if err != nil {
		return err
	}
	if err := releases(ctx, store, projects["sample"].Scope); err != nil {
		return err
	}
	enrollment, err := clusters.CreateEnrollment(ctx, principal)
	if err != nil {
		return err
	}
	identity, err := clusters.Exchange(ctx, "keldock-m1", enrollment.Token)
	if err != nil {
		return err
	}
	client, err := agent.NewClient("http://127.0.0.1:3801", identity.Token, true)
	if err != nil {
		return err
	}
	driver, err := cluster.New(config.Kubeconfig, config.Context, config.RegistryPull, true)
	if err != nil {
		return err
	}
	if _, err := driver.Inspect(ctx); err != nil {
		return err
	}
	publisher, err := artifacts.NewPublisher(artifacts.RegistryConfig{PushRepository: config.RegistryPush, PullRepository: config.RegistryPull, PlainHTTP: true})
	if err != nil {
		return err
	}
	api := &controlplane.API{Store: store, Security: security, Organization: "console-local", Cluster: "keldock-m1", Projects: projects, Domain: "keldock.test", Development: true, Notice: "Synthetic local fixture"}
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	group, active := errgroup.WithContext(ctx)
	group.Go(func() error {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		<-active.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	})
	group.Go(func() error {
		return (controlplane.Worker{Store: store, Publisher: publisher, Organization: "console-local"}).Run(active)
	})
	group.Go(func() error { return (agent.Agent{Client: client, Driver: driver}).Run(active) })
	slog.Info("Synthetic local console fixture ready", "console", "http://127.0.0.1:3800", "accountFile", ".local/console-account.json", "cluster", "k3d-keldock-m1")
	return group.Wait()
}
