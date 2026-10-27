//go:build integration

package access_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
)

func TestSingleUseEnrollmentExclusiveSessionAndRevocation(t *testing.T) {
	store, users, ctx := identityDatabase(t)
	admin, _, _ := administrator(t, users, ctx)
	clusters, err := clusteridentity.NewRepository(store.Pool(), users, "keldock-m1")
	if err != nil {
		t.Fatal(err)
	}
	if err := clusters.Seed(ctx, "Disposable cluster"); err != nil {
		t.Fatal(err)
	}
	grant, err := clusters.CreateEnrollment(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Exchange(ctx, "another-cluster", grant.Token); !errors.Is(err, access.ErrDenied) {
		t.Fatal("enrollment escaped its target")
	}
	var successes atomic.Int32
	identities := make(chan clusteridentity.Identity, 8)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			identity, err := clusters.Exchange(ctx, "keldock-m1", grant.Token)
			if err == nil {
				successes.Add(1)
				identities <- identity
			} else if !errors.Is(err, access.ErrDenied) {
				t.Error(err)
			}
		})
	}
	group.Wait()
	close(identities)
	if successes.Load() != 1 {
		t.Fatal("enrollment was not consumed exactly once")
	}
	identity := <-identities
	sessionOne, sessionTwo := access.NewID("run"), access.NewID("run")
	if _, err := clusters.Authenticate(ctx, identity.Token, sessionOne, "m2-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Authenticate(ctx, identity.Token, sessionTwo, "m2-test"); !errors.Is(err, access.ErrSessionBusy) {
		t.Fatal("two processes acquired the same cluster session")
	}
	if err := clusters.Release(ctx, identity.Token, sessionOne); err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Authenticate(ctx, identity.Token, sessionTwo, "m2-test"); err != nil {
		t.Fatal("graceful restart could not recover the session")
	}
	pending := access.NewToken("kda_")
	rotated, err := clusters.Rotate(ctx, identity.ID, identity.Token, pending, sessionTwo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Authenticate(ctx, identity.Token, sessionTwo, "m2-test"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("old agent credential survived rotation")
	}
	recovered, err := clusters.Rotate(ctx, identity.ID, identity.Token, pending, sessionTwo)
	if err != nil || !recovered.ExpiresAt.Equal(rotated.ExpiresAt) || recovered.Token != pending {
		t.Fatal("uncertain rotation response could not be recovered idempotently")
	}
	info, err := clusters.Information(ctx)
	if err != nil || info.State != "connected" || info.AgentID != identity.ID {
		t.Fatal("cluster connection metadata incorrect")
	}
	if err := clusters.Revoke(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Authenticate(ctx, pending, sessionTwo, "m2-test"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("durable revocation did not reject agent")
	}
	info, err = clusters.Information(ctx)
	if err != nil || info.State != "revoked" {
		t.Fatal("revocation not visible in inventory")
	}
	grant, err = clusters.CreateEnrollment(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := clusters.Exchange(ctx, "keldock-m1", grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == identity.ID || replacement.Generation <= identity.Generation {
		t.Fatal("replacement enrollment reused revoked authority")
	}
	if _, err := clusters.Authenticate(ctx, pending, sessionTwo, "m2-test"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("replacement resurrected an old identity")
	}
}

func TestEnrollmentExpiryAndReadOnlyDenial(t *testing.T) {
	store, users, ctx := identityDatabase(t)
	admin, _, _ := administrator(t, users, ctx)
	clusters, err := clusteridentity.NewRepository(store.Pool(), users, "keldock-m1")
	if err != nil {
		t.Fatal(err)
	}
	if err := clusters.Seed(ctx, "Disposable cluster"); err != nil {
		t.Fatal(err)
	}
	read := admin
	read.Kind = "mcp"
	read.TokenProjects = []string{"sample"}
	if _, err := clusters.CreateEnrollment(ctx, read); !errors.Is(err, access.ErrDenied) {
		t.Fatal("MCP identity created cluster authority")
	}
	if err := clusters.Revoke(ctx, read); !errors.Is(err, access.ErrDenied) {
		t.Fatal("MCP identity revoked cluster authority")
	}
	grant, err := clusters.CreateEnrollment(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE access_enrollments SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, access.TokenHash(grant.Token)); err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Exchange(ctx, "keldock-m1", grant.Token); !errors.Is(err, access.ErrDenied) {
		t.Fatal("expired enrollment accepted")
	}
	grant, err = clusters.CreateEnrollment(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if err := clusters.Revoke(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := clusters.Exchange(ctx, "keldock-m1", grant.Token); !errors.Is(err, access.ErrDenied) {
		t.Fatal("outstanding enrollment survived revocation")
	}
}
