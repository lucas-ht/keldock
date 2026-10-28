//go:build integration

package postgresql_test

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cluster"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/recovery"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func controlPlaneRecovery(t *testing.T, f *fixture, source *v1.ProjectedDeployment, input v1.SubmitRelease) {
	t.Helper()
	// An actual closed listener, rather than a simulated controller condition.
	f.server.Close()
	if err := f.registrar.Step(f.ctx); err == nil {
		t.Fatal("registrar did not observe control-plane outage")
	}
	response := request(t, source.Selection.Hostname, "GET", "/")
	if response.StatusCode != 200 {
		t.Fatal("control-plane outage stopped the running app")
	}
	closeResponse(t, response)
	listener, err := net.Listen("tcp", strings.TrimPrefix(f.publicURL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewUnstartedServer(f.api.Handler())
	if err := f.server.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	f.server.Listener = listener
	f.server.Start()
	t.Cleanup(f.server.Close)
	if err := f.registrar.Step(f.ctx); err != nil {
		t.Fatal("registrar did not reconnect", err)
	}
	t.Log("API listener outage retained application availability; original registrar reconnected")
	private := t.TempDir()
	configFile := filepath.Join(private, "control.json")
	configData, err := json.Marshal(map[string]string{"databaseUrl": f.databaseURL, "organizationId": f.scope.OrganizationID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, configData, 0o600); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"control.json": configFile}
	for i, path := range f.credentialFiles {
		files[[]string{"registrar.json", "database-agent.json"}[i]] = path
	}
	config := recovery.Config{Organization: f.scope.OrganizationID, DatabaseURL: f.databaseURL, Registry: f.registry, Files: files}
	snapshot := filepath.Join(private, "old-snapshot")
	manifest, err := recovery.Backup(f.ctx, config, snapshot)
	if err != nil {
		t.Fatal("control-plane snapshot", err)
	}
	if len(manifest.Artifacts) < 3 {
		t.Fatal("snapshot omitted referenced workload/bundle graphs")
	}
	old := restoreControlPlane(t, f, config, snapshot)
	assertRevoked(t, f, old)
	driver, err := cluster.New(f.kubeconfig, "k3d-keldock-m6", f.registry.PullRepository, true)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := driver.Journal(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.VerifyJournalCoverage(f.ctx, journal); err != nil {
		t.Fatal(err)
	}
	if err := old.ReviewRecovery(f.ctx, f.scope.OrganizationID, journal); err != nil {
		t.Fatal("current checkpoint review", err)
	}
	data, err := driver.DatabaseJournal(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.ReviewDatabaseRecovery(f.ctx, f.scope.OrganizationID, data); err != nil {
		t.Fatal(err)
	}
	// Upgrade the application configuration without rebuilding or changing images.
	input.Source.Event = "push"
	input.Source.Ref = "refs/heads/main"
	input.Source.PullRequest = 0
	input.Source.HeadCommit = ""
	input.Source.Commit = strings.Repeat("b", 40)
	input.Source.RunID++
	component := input.Definition.Components["web"]
	component.Env = map[string]string{"M6_UPGRADE": "2"}
	input.Definition.Components["web"] = component
	receipt, err := f.store.AdmitToDomain(f.ctx, f.scope, "synthetic-local-fixture", "postgresql-upgrade", "keldock.test", input)
	if err != nil {
		t.Fatal(err)
	}
	upgraded := ready(t, f, receipt)
	for name, image := range source.Images {
		if upgraded.Images[name] != image {
			t.Fatal("upgrade rebuilt an admitted image")
		}
	}
	if upgraded.Selection.Databases["data"].Namespace != source.Selection.Databases["data"].Namespace {
		t.Fatal("upgrade replaced retained data")
	}
	response = request(t, source.Selection.Hostname, "GET", "/")
	closeResponse(t, response)
	journal, err = driver.Journal(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.ResumeRecovery(f.ctx, f.scope.OrganizationID, journal); err == nil {
		t.Fatal("stale snapshot resumed over surviving newer generation")
	}
	if fenced, err := old.RecoveryFenced(f.ctx, f.scope.OrganizationID); err != nil || !fenced {
		t.Fatal("stale recovery lost its fence")
	}
	latest := filepath.Join(private, "current-snapshot")
	if _, err := recovery.Backup(f.ctx, config, latest); err != nil {
		t.Fatal(err)
	}
	restored := restoreControlPlane(t, f, config, latest)
	assertRevoked(t, f, restored)
	if err := restored.ReviewDatabaseRecovery(f.ctx, f.scope.OrganizationID, data); err != nil {
		t.Fatal(err)
	}
	recoveredRegistry := config
	recoveredRegistry.Registry.PushRepository = "127.0.0.1:5012/" + strings.SplitN(config.Registry.PushRepository, "/", 2)[1]
	if err := recovery.RestoreBundles(f.ctx, recoveredRegistry, latest); err != nil {
		t.Fatal(err)
	}
	if err := restored.ResumeRecovery(f.ctx, f.scope.OrganizationID, journal); err != nil {
		t.Fatal(err)
	}
	if fenced, err := restored.RecoveryFenced(f.ctx, f.scope.OrganizationID); err != nil || fenced {
		t.Fatal("reviewed recovery remained fenced")
	}
	t.Log("Real pg_dump/pg_restore and OCI export/import passed; recovered credentials revoked; stale snapshot blocked; identical-image upgrade retained data")
}

func restoreControlPlane(t *testing.T, f *fixture, config recovery.Config, snapshot string) *postgres.Store {
	t.Helper()
	name := "keldock_m6_" + access.NewID("restore")
	t.Log("Restoring only into new independent synthetic database:", name)
	if _, err := f.store.Pool().Exec(f.ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(f.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	config.DatabaseURL = parsed.String()
	if err := recovery.Restore(f.ctx, config, snapshot); err != nil {
		t.Fatal(err)
	}
	s, err := postgres.Open(f.ctx, config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(f.ctx); err != nil {
		t.Fatal("restored migration checksums", err)
	}
	if fenced, err := s.RecoveryFenced(f.ctx, f.scope.OrganizationID); err != nil || !fenced {
		t.Fatal("restored state was not fenced")
	}
	return s
}

func assertRevoked(t *testing.T, f *fixture, s *postgres.Store) {
	t.Helper()
	users, err := access.NewRepository(s.Pool(), f.scope.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := users.Session(f.ctx, f.session); err == nil {
		t.Fatal("restored browser session remained valid")
	}
	for _, table := range []string{"access_agents", "access_database_agents"} {
		var count int
		if err := s.Pool().QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE organization_id=$1 AND revoked_at IS NULL", f.scope.OrganizationID).Scan(&count); err != nil || count != 0 {
			t.Fatal("restored agent was not revoked", err)
		}
	}
}
