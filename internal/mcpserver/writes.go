package mcpserver

import (
	"context"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type selectionArguments struct {
	ProjectID     string `json:"projectId" jsonschema:"Authorized project ID"`
	EnvironmentID string `json:"environmentId" jsonschema:"Exact nonproduction target from get_environment"`
	ReleaseID     string `json:"releaseId" jsonschema:"Immutable ID from list_releases; no new build or image input is accepted"`
	Generation    int64  `json:"generation" jsonschema:"Current desired generation observed with get_environment"`
	RequestKey    string `json:"requestKey" jsonschema:"Stable 8-128 character request identity; preserve it for an identical uncertain retry"`
}

type mutationArguments struct {
	ProjectID   string `json:"projectId" jsonschema:"Authorized project ID"`
	OperationID string `json:"operationId" jsonschema:"Current operation ID from get_environment or get_operation"`
	Generation  int64  `json:"generation" jsonschema:"Exact desired generation observed before this intent"`
	RequestKey  string `json:"requestKey" jsonschema:"Stable 8-128 character request identity; preserve it for an identical uncertain retry"`
}

func mutationTool(name, description string) *mcp.Tool {
	yes := true
	return &mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &yes, OpenWorldHint: &yes}}
}

func (adapter *adapter) addWrites(server *mcp.Server) {
	mcp.AddTool(server, tool("list_releases", "List up to 25 CI-admitted immutable releases and source identities in an authorized project; follow nextCursor. Does not expose runtime values or provenance secrets."), func(ctx context.Context, _ *mcp.CallToolRequest, input projectArguments) (*mcp.CallToolResult, any, error) {
		principal, err := adapter.principal(ctx)
		if err != nil {
			return outcome(nil, err)
		}
		result, err := adapter.reads.ListReleases(ctx, principal, input.ProjectID, input.After)
		return outcome(result, err)
	})
	mcp.AddTool(server, mutationTool("deploy_release", "Select an already admitted release for an exact nonproduction target/revision. Requires explicit operation consent. PR head and placement policy must match. Supersedes the previous selection and may rerun release jobs. Returns acceptance, not readiness."), func(ctx context.Context, _ *mcp.CallToolRequest, input selectionArguments) (*mcp.CallToolResult, mutationResult, error) {
		principal, err := adapter.principal(ctx)
		if err != nil {
			return mutationOutcome(v1.Receipt{}, err)
		}
		receipt, err := adapter.writes.Select(ctx, principal, input.ProjectID, v1.ReleaseSelection{ReleaseID: input.ReleaseID, EnvironmentID: input.EnvironmentID, Generation: input.Generation, RequestKey: input.RequestKey})
		return mutationOutcome(receipt, err)
	})
	mcp.AddTool(server, mutationTool("retry_operation", "Retry the current failed or canceled nonproduction deployment using exactly its admitted images and configuration. Requires operation consent, current generation and request identity. May rerun release jobs; returns a durable receipt for the new generation."), func(ctx context.Context, _ *mcp.CallToolRequest, input mutationArguments) (*mcp.CallToolResult, mutationResult, error) {
		principal, err := adapter.principal(ctx)
		if err != nil {
			return mutationOutcome(v1.Receipt{}, err)
		}
		receipt, err := adapter.writes.Retry(ctx, principal, input.ProjectID, v1.OperationMutation{OperationID: input.OperationID, Generation: input.Generation, RequestKey: input.RequestKey})
		return mutationOutcome(receipt, err)
	})
	mcp.AddTool(server, mutationTool("cancel_operation", "Cancel future work for the exact current nonproduction operation/revision. Requires operation consent and stable request identity. Completed effects and running workloads remain; this neither rolls back nor deletes persistent data."), func(ctx context.Context, _ *mcp.CallToolRequest, input mutationArguments) (*mcp.CallToolResult, mutationResult, error) {
		principal, err := adapter.principal(ctx)
		if err != nil {
			return mutationOutcome(v1.Receipt{}, err)
		}
		receipt, err := adapter.writes.Cancel(ctx, principal, input.ProjectID, v1.OperationMutation{OperationID: input.OperationID, Generation: input.Generation, RequestKey: input.RequestKey})
		return mutationOutcome(receipt, err)
	})
}
