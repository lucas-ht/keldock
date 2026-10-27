//go:build integration

package github_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
)

func TestGitHubSetupStateCannotCrossAdministratorOrConfiguration(t *testing.T) {
	f := integration(t)
	status, err := f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := f.service.BeginSetup(f.ctx, f.admin, status.App.Version, "installation")
	if err != nil {
		t.Fatal(err)
	}
	destination, err := url.Parse(setup.URL)
	if err != nil || destination.Host != "github.com" {
		t.Fatal("unsafe installation destination")
	}
	state := destination.Query().Get("state")
	wrong := f.admin
	wrong.User.Version++
	if err := f.service.CompleteSetup(f.ctx, wrong, state, "installation", ""); !errors.Is(err, access.ErrDenied) {
		t.Fatal("stale administrator consumed setup")
	}
	if err := f.service.CompleteSetup(f.ctx, f.admin, state, "installation", ""); err != nil {
		t.Fatal(err)
	}
	if err := f.service.CompleteSetup(f.ctx, f.admin, state, "installation", ""); !errors.Is(err, access.ErrDenied) {
		t.Fatal("setup callback replay accepted")
	}
	setup, err = f.service.BeginSetup(f.ctx, f.admin, status.App.Version, "installation")
	if err != nil {
		t.Fatal(err)
	}
	destination, _ = url.Parse(setup.URL)
	if err := f.service.Repository.BlockInstallation(f.ctx, f.admin, status.App.Version, 701, true); err != nil {
		t.Fatal(err)
	}
	if err := f.service.CompleteSetup(f.ctx, f.admin, destination.Query().Get("state"), "installation", ""); !errors.Is(err, access.ErrConflict) {
		t.Fatal("changed App configuration accepted stale setup")
	}
}

func TestManifestRegistrationKeepsGeneratedSecretsInsideInstallation(t *testing.T) {
	f := integration(t)
	status, err := f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Disconnect(f.ctx, f.admin, status.App.Version); err != nil {
		t.Fatal(err)
	}
	status, err = f.service.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := f.service.BeginSetup(f.ctx, f.admin, status.App.Version, "manifest")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(setup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE KEY") || strings.Contains(string(data), f.app.WebhookSecret) {
		t.Fatal("manifest proposal exposed credentials")
	}
	if setup.Manifest["request_oauth_on_install"] != false || setup.Manifest["public"] != false {
		t.Fatal("manifest expanded account authorization")
	}
	f.remote.mu.Lock()
	f.remote.feedback = func(w http.ResponseWriter, r *http.Request, write func(any)) bool {
		if !strings.HasPrefix(r.URL.Path, "/app-manifests/") {
			return false
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("manifest exchange used unrelated credentials")
		}
		write(map[string]any{"id": f.app.ID, "client_id": f.app.ClientID, "slug": f.app.Slug, "pem": f.app.PrivateKey, "webhook_secret": f.app.WebhookSecret})
		return true
	}
	f.remote.mu.Unlock()
	destination, _ := url.Parse(setup.URL)
	if err := f.service.CompleteSetup(f.ctx, f.admin, destination.Query().Get("state"), "manifest", strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	stored, err := f.service.File.Read()
	if err != nil || stored.PrivateKey != f.app.PrivateKey {
		t.Fatal("generated credentials were not privately persisted")
	}
	status, err = f.service.Status(f.ctx)
	if err != nil || !status.App.Enabled {
		t.Fatal("registered App unavailable")
	}
	if _, err := f.service.Repository.Grant(f.ctx, 100); !errors.Is(err, access.ErrDenied) {
		t.Fatal("manifest callback invented repository authority before sync")
	}
}
