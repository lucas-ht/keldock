// Package mcpserver adapts the shared scoped read service to authenticated MCP over HTTP.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	"github.com/lucas-ht/keldock/internal/inspection"
	"github.com/lucas-ht/keldock/internal/operations"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type credentialKey struct{}

type adapter struct {
	identity *mcpidentity.Repository
	reads    *inspection.Service
	writes   *operations.Service
}

// Handler requires an OAuth bearer even when a browser administrator cookie is present.
// Stateless transport deliberately avoids an authenticated-session cache across requests.
func Handler(identity *mcpidentity.Repository, reads *inspection.Service, writes *operations.Service) http.Handler {
	adapter := &adapter{identity: identity, reads: reads, writes: writes}
	server := adapter.server()
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 64 * 1024, PropagateRequestCancellation: true})
	verify := func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		principal, expiry, err := identity.Authenticate(ctx, token, identity.Resource)
		if errors.Is(err, access.ErrDenied) || errors.Is(err, access.ErrNotFound) {
			return nil, auth.ErrInvalidToken
		}
		if err != nil {
			return nil, fmt.Errorf("authentication unavailable")
		}
		return &auth.TokenInfo{Scopes: strings.Fields(principal.TokenScope), Expiration: expiry, UserID: principal.GrantID}, nil
	}
	authenticated := auth.RequireBearerToken(verify, &auth.RequireBearerTokenOptions{ResourceMetadataURL: identity.Issuer + "/.well-known/oauth-protected-resource/mcp", Scopes: []string{mcpidentity.ReadScope}})(transport)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// OAuth clients may run in a browser. No credentialed CORS or cookie authority is used.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, MCP-Protocol-Version")
		if request.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fields := strings.Fields(request.Header.Get("Authorization"))
		if len(fields) == 2 {
			request = request.WithContext(context.WithValue(request.Context(), credentialKey{}, fields[1]))
		}
		authenticated.ServeHTTP(w, request)
	})
}

func (adapter *adapter) principal(ctx context.Context) (access.Principal, error) {
	token, _ := ctx.Value(credentialKey{}).(string)
	principal, _, err := adapter.identity.Authenticate(ctx, token, adapter.identity.Resource)
	return principal, err
}
