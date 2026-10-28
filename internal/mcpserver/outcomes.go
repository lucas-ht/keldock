package mcpserver

import (
	"errors"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type mutationResult struct {
	Receipt *v1.Receipt  `json:"receipt,omitempty"`
	Error   *errorDetail `json:"error,omitempty"`
}

func toolError(err error) *errorDetail {
	result := &errorDetail{Code: "unavailable", Message: "Request unavailable; retry later using the same request identity"}
	switch {
	case errors.Is(err, access.ErrDenied), errors.Is(err, postgres.ErrDenied):
		result.Code, result.Message = "denied", "This credential or target is outside permitted nonproduction operation scope"
	case errors.Is(err, postgres.ErrNotFound), errors.Is(err, access.ErrNotFound):
		result.Code, result.Message = "not_found", "The requested record was not found in this project"
	case errors.Is(err, access.ErrInvalid):
		result.Code, result.Message = "invalid_request", "Check identifiers, generation and request identity"
	case errors.Is(err, postgres.ErrStale):
		result.Code, result.Message = "stale_revision", "Target revision, PR head, expiry or closure changed; inspect current state"
	case errors.Is(err, postgres.ErrConflict), errors.Is(err, access.ErrConflict):
		result.Code, result.Message = "conflict", "Request identity or operation state conflicts with this intent"
	case errors.Is(err, postgres.ErrLimited):
		result.Code, result.Message = "rate_limited", "Accepted operation limit reached; retry later with the same request identity"
	case errors.Is(err, postgres.ErrQuota):
		result.Code, result.Message = "quota_exceeded", "Preview capacity remains reserved until cleanup is observed"
	}
	return result
}

func mutationOutcome(receipt v1.Receipt, err error) (*mcp.CallToolResult, mutationResult, error) {
	if err != nil {
		return &mcp.CallToolResult{IsError: true}, mutationResult{Error: toolError(err)}, nil
	}
	return nil, mutationResult{Receipt: &receipt}, nil
}
