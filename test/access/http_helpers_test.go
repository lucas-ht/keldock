//go:build integration

package access_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	"github.com/lucas-ht/keldock/internal/controlplane/authentication"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

type identityHTTP struct {
	server     *httptest.Server
	client     *http.Client
	users      *access.Repository
	store      *postgres.Store
	service    *authentication.Service
	invitation access.Invitation
	ctx        context.Context
	csrf       string
}

func newIdentityHTTP(t *testing.T) *identityHTTP {
	t.Helper()
	store, users, ctx := identityDatabase(t)
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	clusters, err := clusteridentity.NewRepository(store.Pool(), users, "keldock-m1")
	if err != nil {
		t.Fatal(err)
	}
	if err := clusters.Seed(ctx, "Disposable cluster"); err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpidentity.NewRepository(store.Pool(), users, origin)
	if err != nil {
		t.Fatal(err)
	}
	if err := mcp.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := authentication.New(users, clusters, mcp, authentication.Config{PublicURL: origin, Development: true, ClusterID: "keldock-m1", Projects: []string{"sample", "restricted"}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	service.Register(mux)
	server.Config.Handler = mux
	server.Start()
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	invitation, err := users.Bootstrap(ctx, "admin@example.test", "Local administrator")
	if err != nil {
		t.Fatal(err)
	}
	return &identityHTTP{server: server, client: client, users: users, store: store, service: service, invitation: invitation, ctx: ctx}
}

func (fixture *identityHTTP) call(t *testing.T, client *http.Client, method, path string, headers map[string]string, input, output any, expected int) *http.Response {
	t.Helper()
	body, ok := input.([]byte)
	if !ok && input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequestWithContext(fixture.ctx, method, fixture.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", fixture.server.URL)
	if fixture.csrf != "" {
		request.Header.Set("X-Keldock-CSRF", fixture.csrf)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected {
		t.Fatalf("%s %s: wanted %d, got %d: %.1024s", method, path, expected, response.StatusCode, data)
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			t.Fatal(err)
		}
	}
	return response
}

func (fixture *identityHTTP) activate(t *testing.T) access.User {
	t.Helper()
	var result struct {
		User access.User `json:"user"`
		CSRF string      `json:"csrfToken"`
	}
	fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/activate", nil, map[string]string{"token": fixture.invitation.Token, "password": "a private local test password"}, &result, http.StatusOK)
	fixture.csrf = result.CSRF
	return result.User
}
