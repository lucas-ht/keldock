//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

func TestAuthenticatedMCPDiscoveryScopesRedactionAndRevocation(t *testing.T) {
	fixture := newAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	restricted := v1.Project{Scope: v1.Scope{OrganizationID: fixture.scope.OrganizationID, ProjectID: "restricted"}, Name: "Restricted", RepositoryID: 43, ClusterID: "keldock-m1"}
	if err := fixture.store.SeedProject(ctx, restricted); err != nil {
		t.Fatal(err)
	}
	fixture.api.Projects["restricted"] = restricted
	principal, _, err := fixture.api.Security.Users.Session(ctx, fixture.session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.api.Security.MCP.SetEnabled(ctx, principal, true, 1); err != nil {
		t.Fatal(err)
	}
	input := applicationInput(t)
	input.Definition.Components["api"].Env["PRIVATE_VALUE"] = "fixture-secret-never-returned"
	receipt := fixture.submit(t, fixture.sign(t, input), "mcp-inspection-release", http.StatusAccepted)
	worker := controlplane.Worker{Store: fixture.store, Publisher: publisher(t), Organization: fixture.scope.OrganizationID}
	if worked, err := worker.Step(ctx); err != nil || !worked {
		t.Fatalf("publish: %v %v", worked, err)
	}
	desired, err := fixture.store.CurrentDesired(ctx, fixture.scope.OrganizationID, "keldock-m1", receipt.EnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	report := v1.Observation{EnvironmentID: receipt.EnvironmentID, DeploymentID: receipt.DeploymentID, Generation: receipt.Generation, Bundle: desired.Bundle, State: "failed", Reason: "ReadinessFailed", Components: []v1.ComponentHealth{{Name: "api", Reason: "ProbeFailed", Message: "fixture-secret-never-returned password=hidden token=kma_example"}}, ObservedAt: time.Now().Add(-time.Minute).UTC()}
	if err := fixture.store.Observe(ctx, fixture.scope.OrganizationID, "keldock-m1", report); err != nil {
		t.Fatal(err)
	}
	oauth, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		NewTokenSource: func(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			expired := *token
			expired.Expiry = time.Now().Add(-time.Minute)
			return config.TokenSource(ctx, &expired), nil
		},
		RedirectURL:                     "http://127.0.0.1:39157/callback",
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "Keldock SDK acceptance", RedirectURIs: []string{"http://127.0.0.1:39157/callback"}, Scope: mcpidentity.ReadScope, GrantTypes: []string{"authorization_code", "refresh_token"}, TokenEndpointAuthMethod: "none"}},
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
			if err != nil {
				return nil, err
			}
			response, err := (&http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
			if err != nil {
				return nil, err
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusSeeOther {
				t.Fatalf("authorize: %d", response.StatusCode)
			}
			location, err := url.Parse(response.Header.Get("Location"))
			if err != nil {
				return nil, err
			}
			id := location.Query().Get("request")
			var consent struct {
				RedirectURL string `json:"redirectUrl"`
			}
			fixture.call(t, http.MethodPost, "/v1/mcp/authorizations/"+id+"/consent", fixture.userHeaders(), map[string]any{"allow": true, "projects": []string{"sample"}}, &consent, http.StatusOK)
			callback, err := url.Parse(consent.RedirectURL)
			if err != nil {
				return nil, err
			}
			return &auth.AuthorizationResult{Code: callback.Query().Get("code"), State: callback.Query().Get("state"), Iss: callback.Query().Get("iss")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "keldock-acceptance", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: fixture.server.URL + "/mcp", OAuthHandler: oauth, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	var refreshes int
	if err := fixture.store.Pool().QueryRow(ctx, `SELECT count(*) FROM access_mcp_tokens t JOIN access_mcp_grants g ON g.id=t.grant_id JOIN access_mcp_clients c ON c.id=g.client_id WHERE c.organization_id=$1 AND t.kind='refresh'`, fixture.scope.OrganizationID).Scan(&refreshes); err != nil || refreshes < 2 {
		t.Fatalf("SDK did not complete token refresh: %d %v", refreshes, err)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatalf("tools: %v %v", tools, err)
	}
	for _, tool := range tools.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatal("MCP exposed a non-read tool")
		}
	}
	for _, call := range []*mcp.CallToolParams{
		{Name: "list_projects", Arguments: map[string]any{}},
		{Name: "list_environments", Arguments: map[string]any{"projectId": "sample"}},
		{Name: "get_environment", Arguments: map[string]any{"projectId": "sample", "environmentId": receipt.EnvironmentID}},
		{Name: "get_operation", Arguments: map[string]any{"projectId": "sample", "operationId": receipt.OperationID}},
		{Name: "get_diagnostics", Arguments: map[string]any{"projectId": "sample", "environmentId": receipt.EnvironmentID}},
	} {
		result, err := session.CallTool(ctx, call)
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %v %v", call.Name, result, err)
		}
		data, _ := json.Marshal(result)
		if strings.Contains(string(data), "fixture-secret-never-returned") || strings.Contains(string(data), "password=hidden") || strings.Contains(string(data), "PRIVATE_VALUE") {
			t.Fatal("MCP leaked private application configuration")
		}
		if call.Name == "list_projects" && strings.Contains(string(data), "restricted") {
			t.Fatal("MCP read inherited its administrator owner's broader project access")
		}
		if call.Name == "get_diagnostics" && (!strings.Contains(string(data), `"stale":true`) || !strings.Contains(string(data), "[redacted]")) {
			t.Fatalf("diagnostic freshness/redaction absent: %s", data)
		}
	}
	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_environments", Arguments: map[string]any{"projectId": "restricted"}})
	if err != nil || !denied.IsError {
		t.Fatalf("cross-project scope was accepted: %v", err)
	}
	encoded, _ := json.Marshal(denied)
	if !strings.Contains(string(encoded), `"code":"denied"`) {
		t.Fatal("MCP error lacked structured classification")
	}
	tokenSource, err := oauth.TokenSource(ctx)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := tokenSource.Token()
	if err != nil {
		t.Fatal(err)
	}
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/cancel", map[string]string{"Authorization": "Bearer " + credential.AccessToken}, nil, nil, http.StatusForbidden)
	// A browser cookie cannot turn an MCP read credential into administrative authority.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fixture.server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", fixture.userHeaders()["Cookie"])
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || !strings.Contains(response.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatal("MCP allowed a cookie or lost OAuth discovery challenge")
	}
	connections, _, err := fixture.api.Security.MCP.Connections(ctx, principal, true, "")
	if err != nil || len(connections) != 1 {
		t.Fatal("UI connection inventory missing")
	}
	if err := fixture.api.Security.MCP.RevokeConnection(ctx, principal, connections[0].ID); err != nil {
		t.Fatal(err)
	}
	// The SDK may ask for a new consent after 401; a raw request proves the revoked bearer is rejected.
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("revoked MCP credential remained usable")
	}
}
