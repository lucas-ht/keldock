//go:build integration

package access_test

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
)

func TestMCPOperationConsentRefreshAndCurrentAuthority(t *testing.T) {
	store, users, ctx := identityDatabase(t)
	admin, _, hash := administrator(t, users, ctx)
	identity, err := mcpidentity.NewRepository(store.Pool(), users, "http://127.0.0.1:3800")
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = identity.SetEnabled(ctx, admin, true, 1); err != nil {
		t.Fatal(err)
	}
	client, err := identity.Register(ctx, "Operation acceptance", []string{"http://127.0.0.1:12345/callback"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := access.NewToken("")
	digest := sha256.Sum256([]byte(verifier))
	input := mcpidentity.AuthorizationInput{ClientID: client.ID, RedirectURI: client.RedirectURIs[0], Challenge: base64.RawURLEncoding.EncodeToString(digest[:]), ChallengeMethod: "S256", Resource: identity.Resource, Scope: mcpidentity.OperationScope, ResponseType: "code"}
	request, err := identity.CreateAuthorization(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	info, err := identity.Authorization(ctx, admin, request)
	if err != nil || info.Scope != mcpidentity.OperationScope {
		t.Fatal("write consent did not show requested scope", err)
	}
	invitation, err := users.Invite(ctx, admin, "viewer@example.test", "Viewer", "viewer", []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := users.Activate(ctx, invitation.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	viewer, _, err := users.Session(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Consent(ctx, viewer, request, true, []string{"sample"}); !errors.Is(err, access.ErrDenied) {
		t.Fatal("viewer delegated write authority", err)
	}
	callback, err := identity.Consent(ctx, admin, request, true, []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(callback)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := identity.Exchange(ctx, client.ID, input.RedirectURI, identity.Resource, parsed.Query().Get("code"), verifier)
	if err != nil || pair.Scope != mcpidentity.OperationScope {
		t.Fatal("write consent was not preserved", err)
	}
	principal, _, err := identity.Authenticate(ctx, pair.AccessToken, identity.Resource)
	if err != nil || !principal.CanOperate("sample") || principal.CanOperate("restricted") || principal.IsAdmin() {
		t.Fatal("operation grant escaped bounded authority", err)
	}
	authorize := func(project string) error {
		tx, e := store.Pool().Begin(ctx)
		if e != nil {
			return e
		}
		e = identity.RequireOperator(ctx, tx, principal, project)
		if rollback := tx.Rollback(ctx); rollback != nil {
			t.Fatal(rollback)
		}
		return e
	}
	if err = authorize("sample"); err != nil {
		t.Fatal("current operator denied", err)
	}
	if err = authorize("restricted"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("cross-project write accepted", err)
	}
	rotated, err := identity.Refresh(ctx, client.ID, identity.Resource, pair.RefreshToken)
	if err != nil || rotated.Scope != pair.Scope {
		t.Fatal("refresh changed explicit scope", err)
	}
	if err = identity.RevokeConnection(ctx, admin, principal.GrantID); err != nil {
		t.Fatal(err)
	}
	if err = authorize("sample"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("revocation did not fence an already authenticated writer", err)
	}
	input.Scope = mcpidentity.ReadScope
	request, err = identity.CreateAuthorization(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	callback, err = identity.Consent(ctx, admin, request, true, []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = url.Parse(callback)
	if err != nil {
		t.Fatal(err)
	}
	pair, err = identity.Exchange(ctx, client.ID, input.RedirectURI, identity.Resource, parsed.Query().Get("code"), verifier)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Refresh(ctx, client.ID, identity.Resource, pair.RefreshToken, mcpidentity.OperationScope); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("read refresh escalated to operation scope", err)
	}
	if rotated, err = identity.Refresh(ctx, client.ID, identity.Resource, pair.RefreshToken); err != nil || rotated.Scope != mcpidentity.ReadScope {
		t.Fatal("failed escalation consumed or expanded the read grant", err)
	}
}
