//go:build integration

package access_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
)

func TestMCPAuthorizationBindingRefreshAndRevocation(t *testing.T) {
	store, users, ctx := identityDatabase(t)
	admin, _, _ := administrator(t, users, ctx)
	mcp, err := mcpidentity.NewRepository(store.Pool(), users, "http://127.0.0.1:3800")
	if err != nil {
		t.Fatal(err)
	}
	if err := mcp.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mcp.Register(ctx, "SDK client", []string{"http://127.0.0.1:12345/callback"}); !errors.Is(err, access.ErrDenied) {
		t.Fatal("disabled endpoint registered a client")
	}
	settings, err := mcp.SetEnabled(ctx, admin, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mcp.Register(ctx, "SDK client", []string{"http://127.0.0.1:12345/callback"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := access.NewToken("")
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	input := mcpidentity.AuthorizationInput{ClientID: client.ID, RedirectURI: client.RedirectURIs[0], State: "client-state", Challenge: challenge, ChallengeMethod: "S256", Resource: mcp.Resource, Scope: mcpidentity.ReadScope, ResponseType: "code"}
	for _, field := range []string{"resource", "redirect", "pkce", "scope"} {
		changed := input
		switch field {
		case "resource":
			changed.Resource = "https://other.example/mcp"
		case "redirect":
			changed.RedirectURI = "https://other.example/callback"
		case "pkce":
			changed.ChallengeMethod = "plain"
		case "scope":
			changed.Scope = "admin"
		}
		if _, err := mcp.CreateAuthorization(ctx, changed); err == nil {
			t.Fatalf("invalid %s binding accepted", field)
		}
	}
	authorize := func() (mcpidentity.TokenPair, string) {
		t.Helper()
		request, err := mcp.CreateAuthorization(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		info, err := mcp.Authorization(ctx, admin, request)
		if err != nil || info.ClientID != client.ID || info.Scope != mcpidentity.ReadScope {
			t.Fatal("consent information incorrect")
		}
		callback, err := mcp.Consent(ctx, admin, request, true, []string{"sample"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mcp.Consent(ctx, admin, request, true, []string{"sample"}); !errors.Is(err, access.ErrDenied) {
			t.Fatal("consent replay accepted")
		}
		parsed, err := url.Parse(callback)
		if err != nil || parsed.Query().Get("state") != input.State || parsed.Query().Get("iss") != mcp.Issuer {
			t.Fatal("callback state or issuer lost")
		}
		code := parsed.Query().Get("code")
		if _, err := mcp.Exchange(ctx, client.ID, input.RedirectURI, mcp.Resource, code, strings.Repeat("x", 43)); !errors.Is(err, access.ErrDenied) {
			t.Fatal("invalid PKCE proof exchanged code")
		}
		pair, err := mcp.Exchange(ctx, client.ID, input.RedirectURI, mcp.Resource, code, verifier)
		if err != nil {
			t.Fatal(err)
		}
		return pair, code
	}
	pair, _ := authorize()
	principal, _, err := mcp.Authenticate(ctx, pair.AccessToken, mcp.Resource)
	if err != nil || !principal.CanRead("sample") || principal.CanRead("restricted") || principal.IsAdmin() || principal.CanOperate("sample") {
		t.Fatal("MCP grant escaped read/project boundaries")
	}
	if _, _, err := mcp.Authenticate(ctx, pair.AccessToken, "https://other.example/mcp"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("wrong audience accepted")
	}
	rotated, err := mcp.Refresh(ctx, client.ID, mcp.Resource, pair.RefreshToken)
	if err != nil || rotated.RefreshToken == pair.RefreshToken {
		t.Fatal("refresh token not rotated")
	}
	if _, err := mcp.Refresh(ctx, client.ID, mcp.Resource, pair.RefreshToken); !errors.Is(err, access.ErrDenied) {
		t.Fatal("refresh replay accepted")
	}
	if _, _, err := mcp.Authenticate(ctx, rotated.AccessToken, mcp.Resource); !errors.Is(err, access.ErrDenied) {
		t.Fatal("refresh replay did not revoke connection family")
	}
	pair, code := authorize()
	if _, err := mcp.Exchange(ctx, client.ID, input.RedirectURI, mcp.Resource, code, verifier); !errors.Is(err, access.ErrDenied) {
		t.Fatal("code replay accepted")
	}
	if _, _, err := mcp.Authenticate(ctx, pair.AccessToken, mcp.Resource); !errors.Is(err, access.ErrDenied) {
		t.Fatal("code replay did not revoke issued tokens")
	}
	pair, _ = authorize()
	principal, _, err = mcp.Authenticate(ctx, pair.AccessToken, mcp.Resource)
	if err != nil {
		t.Fatal(err)
	}
	connections, _, err := mcp.Connections(ctx, admin, false, "")
	if err != nil || len(connections) != 3 {
		t.Fatal("UI connection inventory incorrect")
	}
	data, err := json.Marshal(connections)
	if err != nil || strings.Contains(string(data), pair.AccessToken) || strings.Contains(string(data), pair.RefreshToken) {
		t.Fatal("UI inventory exposed a credential")
	}
	if err := mcp.RevokeConnection(ctx, admin, principal.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mcp.Authenticate(ctx, pair.AccessToken, mcp.Resource); !errors.Is(err, access.ErrDenied) {
		t.Fatal("UI revocation did not invalidate access token")
	}
	pair, _ = authorize()
	if _, err := mcp.SetEnabled(ctx, admin, false, settings.Version); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mcp.Authenticate(ctx, pair.AccessToken, mcp.Resource); !errors.Is(err, access.ErrDenied) {
		t.Fatal("endpoint disable left active authorization")
	}
}
