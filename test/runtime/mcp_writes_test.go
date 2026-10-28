//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/operations"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPBoundedWritesReceiptsSessionLossAndPolicyParity(t *testing.T) {
	fixture := newAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	principal, _, err := fixture.api.Security.Users.Session(ctx, fixture.session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.api.Security.MCP.SetEnabled(ctx, principal, true, 1); err != nil {
		t.Fatal(err)
	}
	input := applicationInput(t)
	input.Definition.Components["api"].Env["PRIVATE_VALUE"] = "m4-secret-do-not-return"
	original := fixture.submit(t, fixture.sign(t, input), "m4-original-admission", http.StatusAccepted)
	session, oauth := sdkMCP(t, fixture, ctx, mcpidentity.OperationScope)
	call := func(name string, args map[string]any, code string) v1.Receipt {
		t.Helper()
		result, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		data, e := json.Marshal(result.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(data), "m4-secret-do-not-return") {
			t.Fatal("MCP leaked admitted runtime values")
		}
		var value struct {
			Receipt *v1.Receipt `json:"receipt"`
			Error   *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if e = json.Unmarshal(data, &value); e != nil {
			t.Fatal(e)
		}
		if code != "" {
			if !result.IsError || value.Error == nil || value.Error.Code != code {
				t.Fatalf("%s: expected %s: %s", name, code, data)
			}
			return v1.Receipt{}
		}
		if result.IsError || value.Receipt == nil || value.Receipt.OperationID == "" {
			t.Fatalf("%s: receipt missing: %s", name, data)
		}
		return *value.Receipt
	}
	selectArgs := map[string]any{"projectId": "sample", "environmentId": original.EnvironmentID, "releaseId": original.ReleaseID, "generation": original.Generation, "requestKey": "m4-select-admitted"}
	selected := call("deploy_release", selectArgs, "")
	if selected.ReleaseID != original.ReleaseID || selected.Generation != original.Generation+1 {
		t.Fatal("selection changed admitted release or generation")
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	session, err = mcp.NewClient(&mcp.Implementation{Name: "resume-acceptance", Version: "1.0.0"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: fixture.server.URL + "/mcp", OAuthHandler: oauth, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	if replay := call("deploy_release", selectArgs, ""); replay != selected {
		t.Fatal("session loss duplicated selection")
	}
	stale := map[string]any{"projectId": "sample", "environmentId": original.EnvironmentID, "releaseId": original.ReleaseID, "generation": original.Generation, "requestKey": "m4-stale-new-intent"}
	call("deploy_release", stale, "stale_revision")
	denied := map[string]any{"projectId": "restricted", "environmentId": original.EnvironmentID, "releaseId": original.ReleaseID, "generation": selected.Generation, "requestKey": "m4-denied-project"}
	call("deploy_release", denied, "denied")
	injected := map[string]any{"projectId": "sample", "environmentId": original.EnvironmentID, "releaseId": original.ReleaseID, "generation": selected.Generation, "requestKey": "m4-reject-new-image", "images": input.Images}
	rejected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "deploy_release", Arguments: injected})
	if err != nil || !rejected.IsError {
		t.Fatal("tool accepted new provenance/image inputs", err)
	}
	cancelArgs := map[string]any{"projectId": "sample", "operationId": selected.OperationID, "generation": selected.Generation, "requestKey": "m4-cancel-selected"}
	canceled := call("cancel_operation", cancelArgs, "")
	if canceled.OperationID != selected.OperationID {
		t.Fatal("cancel targeted another operation")
	}
	if again := call("cancel_operation", cancelArgs, ""); again != canceled {
		t.Fatal("cancel receipt not durable")
	}
	retryArgs := map[string]any{"projectId": "sample", "operationId": selected.OperationID, "generation": selected.Generation, "requestKey": "m4-retry-canceled"}
	retried := call("retry_operation", retryArgs, "")
	if retried.ReleaseID != original.ReleaseID || retried.Generation != selected.Generation+1 {
		t.Fatal("retry changed admitted inputs")
	}
	// All adapters share the production exclusion, even for an administrator-owned grant.
	if _, err = fixture.store.Pool().Exec(ctx, `UPDATE environments SET kind='production' WHERE id=$1`, original.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	production := map[string]any{"projectId": "sample", "environmentId": original.EnvironmentID, "releaseId": original.ReleaseID, "generation": retried.Generation, "requestKey": "m4-deny-production"}
	call("deploy_release", production, "denied")
	opArgs := map[string]any{"projectId": "sample", "operationId": retried.OperationID, "generation": retried.Generation, "requestKey": "m4-deny-prod-cancel"}
	call("cancel_operation", opArgs, "denied")
	opArgs["requestKey"] = "m4-deny-prod-retry"
	call("retry_operation", opArgs, "denied")
	headers := fixture.userHeaders()
	headers["Idempotency-Key"] = "m4-ui-deny-prod"
	fixture.call(t, http.MethodPost, "/v1/projects/sample/environments/"+original.EnvironmentID+"/deployments", headers, map[string]any{"releaseId": original.ReleaseID, "generation": retried.Generation}, nil, http.StatusForbidden)
	if _, err = fixture.store.Pool().Exec(ctx, `UPDATE environments SET kind='staging' WHERE id=$1`, original.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	tokenSource, err := oauth.TokenSource(ctx)
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		t.Fatal(err)
	}
	writer, _, err := fixture.api.Security.MCP.Authenticate(ctx, token.AccessToken, fixture.api.Security.MCP.Resource)
	if err != nil {
		t.Fatal(err)
	}
	actor := "mcp:user:" + writer.User.ID + ":grant:" + writer.GrantID
	if _, err = fixture.store.Pool().Exec(ctx, `INSERT INTO operation_budgets(organization_id,actor,window_start,count) SELECT $1,$2,date_trunc('minute',clock_timestamp())+n*interval '1 minute',30 FROM generate_series(0,1) AS minutes(n) ON CONFLICT(organization_id,actor,window_start) DO UPDATE SET count=30`, fixture.scope.OrganizationID, actor); err != nil {
		t.Fatal(err)
	}
	limited := map[string]any{"projectId": "sample", "environmentId": original.EnvironmentID, "releaseId": original.ReleaseID, "generation": retried.Generation, "requestKey": "m4-accepted-write-budget"}
	call("deploy_release", limited, "rate_limited")
	if replay := call("deploy_release", selectArgs, ""); replay != selected {
		t.Fatal("rate limit blocked recovery of accepted work")
	}
	if err = fixture.api.Security.MCP.RevokeConnection(ctx, principal, writer.GrantID); err != nil {
		t.Fatal(err)
	}
	service := operations.Service{Store: fixture.store, Users: fixture.api.Security.Users, MCP: fixture.api.Security.MCP, Projects: fixture.api.Projects, Organization: fixture.scope.OrganizationID}
	if _, err = service.Select(ctx, writer, "sample", v1.ReleaseSelection{EnvironmentID: original.EnvironmentID, ReleaseID: original.ReleaseID, Generation: retried.Generation, RequestKey: "m4-cached-principal-revoked"}); !errors.Is(err, access.ErrDenied) {
		t.Fatal("revoked cached principal wrote desired state", err)
	}
}
