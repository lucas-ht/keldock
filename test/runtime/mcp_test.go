//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	seedMCPResource(t, ctx, fixture, receipt.EnvironmentID)
	session, oauth := sdkMCP(t, fixture, ctx, mcpidentity.ReadScope)
	defer func() { _ = session.Close() }()
	var refreshes int
	if err := fixture.store.Pool().QueryRow(ctx, `SELECT count(*) FROM access_mcp_tokens t JOIN access_mcp_grants g ON g.id=t.grant_id JOIN access_mcp_clients c ON c.id=g.client_id WHERE c.organization_id=$1 AND t.kind='refresh'`, fixture.scope.OrganizationID).Scan(&refreshes); err != nil || refreshes < 2 {
		t.Fatalf("SDK did not complete token refresh: %d %v", refreshes, err)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 10 {
		t.Fatalf("tools: %v %v", tools, err)
	}
	allowed := map[string]bool{"list_resources": true, "list_projects": true, "list_environments": true, "get_environment": true, "get_operation": true, "get_diagnostics": true, "list_releases": true, "deploy_release": false, "retry_operation": false, "cancel_operation": false}
	for _, tool := range tools.Tools {
		read, exists := allowed[tool.Name]
		if !exists || tool.Annotations == nil || tool.Annotations.ReadOnlyHint != read {
			t.Fatal("unexpected MCP authority surface", tool.Name)
		}
	}
	for _, call := range []*mcp.CallToolParams{
		{Name: "list_projects", Arguments: map[string]any{}},
		{Name: "list_resources", Arguments: map[string]any{"projectId": "sample"}},
		{Name: "list_releases", Arguments: map[string]any{"projectId": "sample"}},
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
		if call.Name == "list_resources" {
			for _, private := range []string{"private-source-identity", "private-archive-reference", "private-database-namespace"} {
				if strings.Contains(string(data), private) {
					t.Fatal("MCP exposed private resource configuration")
				}
			}
			if !strings.Contains(string(data), `"kind":"postgresql"`) || !strings.Contains(string(data), `"walHealthy":true`) || !strings.Contains(string(data), `"stale":true`) {
				t.Fatal("MCP omitted retained resource recovery/freshness evidence")
			}
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
	deniedResources, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources", Arguments: map[string]any{"projectId": "restricted"}})
	if err != nil || !deniedResources.IsError {
		t.Fatal("MCP resource read crossed project scope")
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
