//go:build integration

package postgresql_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/cluster"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/lucas-ht/keldock/internal/controlplane/authentication"
	"github.com/lucas-ht/keldock/internal/database"
	databasestore "github.com/lucas-ht/keldock/internal/database/store"
	"github.com/lucas-ht/keldock/internal/databaseagent"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

type fixture struct {
	ctx             context.Context
	store           *postgres.Store
	scope           v1.Scope
	policy          database.Policy
	provisioner     databaseagent.Agent
	registrar       agent.Agent
	worker          controlplane.Worker
	kubeconfig      string
	principal       access.Principal
	databaseStore   *databasestore.Store
	publicURL       string
	session         string
	databaseURL     string
	registry        artifacts.RegistryConfig
	api             *controlplane.API
	server          *httptest.Server
	credentialFiles []string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	t.Cleanup(cancel)
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		version, err := exec.CommandContext(ctx, tool, "--version").Output()
		if err != nil || !strings.Contains(string(version), "(PostgreSQL) 18.") {
			t.Fatalf("PostgreSQL acceptance requires %s 18.x on PATH before creating fixtures", tool)
		}
	}
	data, err := os.ReadFile("../../.local/runtime.json")
	if err != nil {
		t.Fatal("run task local:up")
	}
	var config struct {
		DatabaseURL  string `json:"databaseUrl"`
		RegistryPush string `json:"registryPush"`
		RegistryPull string `json:"registryPull"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(config.DatabaseURL)
	if err != nil || parsed.Host != "127.0.0.1:55432" || parsed.Path != "/keldock" {
		t.Fatal("refusing PostgreSQL acceptance outside owned local target")
	}
	kubeconfig, err := filepath.Abs("../../.local/m6/kubeconfig")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Target: disposable k3d-keldock-m6; database 127.0.0.1:55432/keldock; no cloud account/backend")
	admin, err := postgres.Open(ctx, config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := "keldock_m6_" + access.NewID("test")
	t.Log("Creating independent synthetic control-plane database:", databaseName)
	if _, err := admin.Pool().Exec(ctx, "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	admin.Close()
	parsed.Path = "/" + databaseName
	config.DatabaseURL = parsed.String()
	s, err := postgres.Open(ctx, config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	scope := v1.Scope{OrganizationID: access.NewID("m6"), ProjectID: "sample"}
	project := v1.Project{Scope: scope, Name: "PostgreSQL acceptance", RepositoryID: 42, ClusterID: "keldock-m6"}
	if err := s.SeedProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	policy := database.Policy{Scope: scope, ClusterID: project.ClusterID, Limit: 4, Profiles: map[string]database.Profile{"development": {StorageClass: "local-path", StorageSize: "1Gi"}}}
	if err := s.SeedDatabasePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	users, err := access.NewRepository(s.Pool(), scope.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := users.Bootstrap(ctx, "admin@example.test", "Synthetic alpha operator")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("synthetic alpha operator password")
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := users.Activate(ctx, grant.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	principal, _, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := clusteridentity.NewRepository(s.Pool(), users, project.ClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Seed(ctx, project.ClusterID); err != nil {
		t.Fatal(err)
	}
	dbidentity, err := clusteridentity.NewDatabaseRepository(s.Pool(), users, project.ClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbidentity.Seed(ctx, project.ClusterID); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	mcp, err := mcpidentity.NewRepository(s.Pool(), users, origin)
	if err != nil {
		t.Fatal(err)
	}
	if err := mcp.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	security, err := authentication.New(users, identities, mcp, authentication.Config{PublicURL: origin, Development: true, ClusterID: project.ClusterID, Projects: []string{scope.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	security.Database = dbidentity
	databaseStore := &databasestore.Store{Pool: s.Pool(), Users: users, Policies: map[string]database.Policy{scope.ProjectID: policy}}
	api := &controlplane.API{Store: s, Organization: scope.OrganizationID, Cluster: project.ClusterID, Domain: "keldock.test", Projects: map[string]v1.Project{scope.ProjectID: project}, Security: security, Development: true, Databases: databaseStore}
	server.Config.Handler = api.Handler()
	server.Start()
	t.Cleanup(server.Close)
	directory := t.TempDir()
	agentPath := filepath.Join(directory, "registrar.json")
	dbPath := filepath.Join(directory, "database.json")
	agentGrant, err := identities.CreateEnrollment(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.Enroll(ctx, origin, project.ClusterID, agentGrant.Token, agentPath, true); err != nil {
		t.Fatal(err)
	}
	dbGrant, err := dbidentity.CreateEnrollment(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.EnrollDatabase(ctx, origin, project.ClusterID, dbGrant.Token, dbPath, true); err != nil {
		t.Fatal(err)
	}
	client, err := agent.ClientFromFile(origin, agentPath, true)
	if err != nil {
		t.Fatal(err)
	}
	dbclient, err := agent.DatabaseClientFromFile(origin, dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := cluster.New(kubeconfig, "k3d-keldock-m6", config.RegistryPull, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	dbdriver, err := databaseagent.New(kubeconfig, "k3d-keldock-m6")
	if err != nil {
		t.Fatal(err)
	}
	if err := dbdriver.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	publisher, err := artifacts.NewPublisher(artifacts.RegistryConfig{PushRepository: config.RegistryPush, PullRepository: config.RegistryPull, PlainHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{registry: artifacts.RegistryConfig{PushRepository: config.RegistryPush, PullRepository: config.RegistryPull, PlainHTTP: true}, api: api, server: server, credentialFiles: []string{agentPath, dbPath}, ctx: ctx, store: s, scope: scope, policy: policy, kubeconfig: kubeconfig, principal: principal, databaseStore: databaseStore, publicURL: origin, session: token, databaseURL: config.DatabaseURL, provisioner: databaseagent.Agent{Client: dbclient, Driver: dbdriver, Policies: map[string]database.Policy{scope.ProjectID: policy}}, registrar: agent.Agent{Client: client, Driver: driver}, worker: controlplane.Worker{Store: s, Organization: scope.OrganizationID, Publisher: publisher}}
}
