//go:build integration

package github_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestCurrentGitHubDelegationRenameTransferAndDisconnect(t *testing.T) {
	f := integration(t)
	if _, err := f.service.AuthorizeRepository(f.ctx, 100, 42, "owner/sample"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AuthorizeRepository(f.ctx, 101, 42, "owner/other"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("unselected repository accepted")
	}
	f.remote.mu.Lock()
	f.remote.name = "owner/renamed"
	f.remote.mu.Unlock()
	if _, err := f.service.AuthorizeRepository(f.ctx, 100, 42, "owner/sample"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("stale name accepted")
	}
	if _, err := f.service.AuthorizeRepository(f.ctx, 100, 42, "owner/renamed"); err != nil {
		t.Fatal("stable repository identity did not survive rename")
	}
	f.remote.mu.Lock()
	f.remote.owner = 99
	f.remote.mu.Unlock()
	if _, err := f.service.AuthorizeRepository(f.ctx, 100, 42, "owner/renamed"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("unapproved repository transfer accepted")
	}
	f.remote.mu.Lock()
	f.remote.owner = 42
	f.remote.mu.Unlock()
	state, err := f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Repository.BlockInstallation(f.ctx, f.admin, state.App.Version, 701, true); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Repository.Grant(f.ctx, 100); !errors.Is(err, access.ErrDenied) {
		t.Fatal("synchronization resurrected local disconnect")
	}
	if err := f.service.Repository.BlockInstallation(f.ctx, f.admin, state.App.Version, 701, false); !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale configuration version accepted")
	}
	state, err = f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Repository.BlockInstallation(f.ctx, f.admin, state.App.Version, 701, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Repository.Grant(f.ctx, 100); err != nil {
		t.Fatal(err)
	}
	state, err = f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Disconnect(f.ctx, f.admin, state.App.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Repository.Grant(f.ctx, 100); !errors.Is(err, access.ErrDenied) {
		t.Fatal("disconnected App retained authority")
	}
	if err := f.service.File.Write(f.app); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Initialize(f.ctx); !errors.Is(err, access.ErrDenied) {
		t.Fatal("restart resurrected a disabled App")
	}
}

func signature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		panic(err)
	}
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookPersistenceLeaseFencingAndImmediateRevocation(t *testing.T) {
	f := integration(t)
	payload := []byte(`{"action":"removed","installation":{"id":701},"repositories_removed":[{"id":100}]}`)
	if _, err := f.service.Receive(f.ctx, "bad-signature", "installation_repositories", "sha256=00", payload); !errors.Is(err, access.ErrDenied) {
		t.Fatal("unsigned delivery accepted")
	}
	changed, err := f.service.Receive(f.ctx, "delivery-1", "installation_repositories", signature(f.app.WebhookSecret, payload), payload)
	if err != nil || !changed {
		t.Fatal(err)
	}
	if _, err := f.service.Repository.Grant(f.ctx, 100); !errors.Is(err, access.ErrDenied) {
		t.Fatal("removed repository still authorized before worker processing")
	}
	changed, err = f.service.Receive(f.ctx, "delivery-1", "installation_repositories", signature(f.app.WebhookSecret, payload), payload)
	if err != nil || changed {
		t.Fatal("duplicate delivery changed work")
	}
	other := []byte(`{"action":"added"}`)
	if _, err := f.service.Receive(f.ctx, "delivery-1", "installation_repositories", signature(f.app.WebhookSecret, other), other); !errors.Is(err, access.ErrConflict) {
		t.Fatal("conflicting replay accepted")
	}
	first, err := f.service.Repository.ClaimEvent(f.ctx)
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if next, err := f.service.Repository.ClaimEvent(f.ctx); err != nil || next != nil {
		t.Fatal("active event lease was stolen")
	}
	if _, err := f.service.Product.Pool().Exec(f.ctx, `UPDATE github_events SET lease_until=now()-interval '1 second' WHERE organization_id=$1 AND delivery_id=$2`, f.users.Organization, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Repository.ClaimEvent(f.ctx)
	if err != nil || second == nil || first.Lease == second.Lease {
		t.Fatal("expired event lease did not recover")
	}
	if err := f.service.Repository.FinishEvent(f.ctx, *first, true, time.Second); !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale worker acknowledged newer work")
	}
	if err := f.service.Repository.FinishEvent(f.ctx, *second, true, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestMissedPullClosureRecoveryAndCredentialPrivacy(t *testing.T) {
	f := integration(t)
	f.remote.mu.Lock()
	f.remote.pull.State = "closed"
	f.remote.pull.UpdatedAt = f.remote.pull.UpdatedAt.Add(time.Second)
	f.remote.mu.Unlock()
	if err := f.service.Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	var closed bool
	if err := f.service.Product.Pool().QueryRow(f.ctx, `SELECT closed FROM pull_requests WHERE organization_id=$1 AND project_id='sample' AND number=1`, f.users.Organization).Scan(&closed); err != nil || !closed {
		t.Fatal("missed closure was not recovered")
	}
	state, err := f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE KEY") || strings.Contains(string(data), f.app.WebhookSecret) {
		t.Fatal("credential material returned in status")
	}
	wrong := f.admin
	wrong.User.Version++
	if err := f.service.Configure(f.ctx, wrong, state.App.Version, f.app); !errors.Is(err, access.ErrDenied) {
		t.Fatal("stale administrator changed App credentials")
	}
	source := v1.Source{RepositoryID: 100, OwnerID: 42, Repository: "owner/sample", PullRequest: 1}
	if err := f.service.AuthorizeRelease(f.ctx, source); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Product.Pool().QueryRow(f.ctx, `SELECT closed FROM pull_requests WHERE organization_id=$1 AND project_id='sample' AND number=1`, f.users.Organization).Scan(&closed); err != nil || !closed {
		t.Fatal("read refresh reopened a closed pull request")
	}
}
