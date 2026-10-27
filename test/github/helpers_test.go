//go:build integration

package github_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/githubapp"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

type remote struct {
	mu        sync.Mutex
	selected  bool
	suspended bool
	name      string
	owner     int64
	pull      client.PullRequest
}

type fixture struct {
	service *githubapp.Service
	users   *access.Repository
	admin   access.Principal
	app     credentials.App
	remote  *remote
	ctx     context.Context
}

func integration(t *testing.T) fixture {
	t.Helper()
	data, err := os.ReadFile("../../.local/runtime.json")
	if err != nil {
		t.Fatal("run task local:up first")
	}
	var configuration struct {
		DatabaseURL string `json:"databaseUrl"`
	}
	if err := json.Unmarshal(data, &configuration); err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(configuration.DatabaseURL)
	if err != nil || endpoint.Host != "127.0.0.1:55432" || endpoint.Path != "/keldock" {
		t.Fatal("refusing GitHub tests outside disposable keldock-m1 PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	product, err := postgres.Open(ctx, configuration.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(product.Close)
	if err := product.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	organization := access.NewID("github")
	project := v1.Project{Scope: v1.Scope{OrganizationID: organization, ProjectID: "sample"}, Name: "Sample", RepositoryID: 100, ClusterID: "keldock-m1"}
	if err := product.SeedProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	users, err := access.NewRepository(product.Pool(), organization)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := users.Bootstrap(ctx, "admin@example.test", "Administrator")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("a private GitHub integration test password")
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := users.Activate(ctx, invitation.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	principal, _, err := users.Session(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	app := credentials.App{ID: 501, ClientID: "test-client", Slug: "keldock-test", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), WebhookSecret: strings.Repeat("a", 64)}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	private := credentials.File{Path: filepath.Join(directory, "app.json")}
	if err := private.Write(app); err != nil {
		t.Fatal(err)
	}
	state := &remote{selected: true, name: "owner/sample", owner: 42}
	state.pull.Number = 1
	state.pull.State = "open"
	state.pull.UpdatedAt = time.Now().UTC()
	state.pull.Head.SHA = strings.Repeat("a", 40)
	state.pull.Head.Repo.ID = 100
	server := httptest.NewServer(http.HandlerFunc(state.handler))
	t.Cleanup(server.Close)
	service, err := githubapp.New(product, users, githubapp.Config{CredentialsFile: private.Path, PublicURL: "http://127.0.0.1:3800", Development: true, Projects: map[string]v1.Project{"sample": project}, RepositoryOwners: map[int64]int64{100: 42}, Client: client.Options{BaseURL: server.URL, Development: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	return fixture{service: service, users: users, admin: principal, app: app, remote: state, ctx: ctx}
}

func (remote *remote) handler(w http.ResponseWriter, r *http.Request) {
	remote.mu.Lock()
	defer remote.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(value any) {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			return
		}
	}
	permissions := githubapp.RequiredPermissions()
	installation := client.Installation{ID: 701, AppID: 501, Account: client.Account{ID: 42, Login: "owner"}, Permissions: permissions}
	if remote.suspended {
		now := time.Now()
		installation.SuspendedAt = &now
	}
	repository := client.Repository{ID: 100, FullName: remote.name, Owner: client.Account{ID: remote.owner, Login: "owner"}, Private: true, DefaultBranch: "main"}
	switch {
	case r.URL.Path == "/app":
		write(client.App{ID: 501, Slug: "keldock-test", ClientID: "test-client", Permissions: permissions})
	case r.URL.Path == "/app/installations":
		write([]client.Installation{installation})
	case r.URL.Path == "/app/installations/701/access_tokens":
		write(map[string]any{"token": "installation-test-credential", "expires_at": time.Now().Add(time.Hour)})
	case r.URL.Path == "/installation/repositories":
		items := []client.Repository{}
		if remote.selected {
			items = append(items, repository)
		}
		write(map[string]any{"repositories": items})
	case r.URL.Path == "/repositories/100":
		if !remote.selected || remote.suspended {
			w.WriteHeader(404)
			return
		}
		write(repository)
	case r.URL.Path == "/repos/"+remote.name+"/pulls":
		items := []client.PullRequest{}
		if remote.pull.State == "open" {
			items = append(items, remote.pull)
		}
		write(items)
	case r.URL.Path == "/repos/"+remote.name+"/pulls/1":
		write(remote.pull)
	default:
		w.WriteHeader(404)
	}
}
