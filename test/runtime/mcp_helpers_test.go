//go:build integration

package runtime_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

func sdkMCP(t *testing.T, fixture *apiFixture, ctx context.Context, scope string) (*mcp.ClientSession, *auth.AuthorizationCodeHandler) {
	t.Helper()
	oauth, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		ScopeFilter: func(_ []string) []string { return strings.Fields(scope) },
		NewTokenSource: func(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			expired := *token
			expired.Expiry = time.Now().Add(-time.Minute)
			return config.TokenSource(ctx, &expired), nil
		},
		RedirectURL:                     "http://127.0.0.1:39157/callback",
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "Keldock SDK acceptance", RedirectURIs: []string{"http://127.0.0.1:39157/callback"}, Scope: scope, GrantTypes: []string{"authorization_code", "refresh_token"}, TokenEndpointAuthMethod: "none"}},
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
	return session, oauth
}
