//go:build integration

package access_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
)

func TestMCPOperationAuthorityUsesRevocationLockOrder(t *testing.T) {
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
	client, err := identity.Register(ctx, "Lock ordering", []string{"http://127.0.0.1:12345/callback"})
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := users.Invite(ctx, admin, "operator@example.test", "Operator", "operator", []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := users.Activate(ctx, invitation.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	operator, _, err := users.Session(ctx, session)
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
	callback, err := identity.Consent(ctx, operator, request, true, []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(callback)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := identity.Exchange(ctx, client.ID, input.RedirectURI, identity.Resource, parsed.Query().Get("code"), verifier)
	if err != nil {
		t.Fatal(err)
	}
	principal, _, err := identity.Authenticate(ctx, pair.AccessToken, identity.Resource)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct{ name, query, id string }{
		{"client", `SELECT id FROM access_mcp_clients WHERE id=$1 FOR UPDATE`, client.ID},
		{"endpoint", `SELECT organization_id FROM access_mcp_settings WHERE organization_id=$1 FOR UPDATE`, users.Organization},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			revocation, err := store.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = revocation.Rollback(context.Background()) }()
			if _, err = users.RequireHuman(ctx, revocation, admin); err != nil {
				t.Fatal(err)
			}
			var id string
			if err = revocation.QueryRow(ctx, test.query, test.id).Scan(&id); err != nil {
				t.Fatal(err)
			}
			mutation, err := store.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				result := identity.RequireOperator(ctx, mutation, principal, "sample")
				_ = mutation.Rollback(context.Background())
				done <- result
			}()
			// Wait for the actual database dependency, not an assumed goroutine schedule.
			for {
				var blocked bool
				err = store.Pool().QueryRow(ctx, `SELECT $2::int=ANY(pg_blocking_pids($1::int))`, mutation.Conn().PgConn().PID(), revocation.Conn().PgConn().PID()).Scan(&blocked)
				if err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				time.Sleep(time.Millisecond)
			}
			// Real revocation holds the endpoint/client before updating its grants. The
			// waiting operation must not hold a grant that would create the reverse edge.
			err = revocation.QueryRow(ctx, `SELECT id FROM access_mcp_grants WHERE id=$1 FOR UPDATE NOWAIT`, principal.GrantID).Scan(&id)
			if err != nil {
				t.Fatalf("mutation locked grant before %s revocation: %v", test.name, err)
			}
			if err = revocation.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
