//go:build integration

package access_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/agent"
)

func TestAgentCredentialFileRecoversLostRotationAndRevocation(t *testing.T) {
	fixture := newIdentityHTTP(t)
	fixture.activate(t)
	base, _ := url.Parse(fixture.server.URL)
	cookies := fixture.client.Jar.Cookies(base)
	if len(cookies) != 1 {
		t.Fatal("expected one browser session")
	}
	principal, _, err := fixture.service.Users.Session(fixture.ctx, cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := fixture.service.Clusters.CreateEnrollment(fixture.ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := agent.Enroll(fixture.ctx, fixture.server.URL, "keldock-m1", enrollment.Token, path, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("agent credential was not owner-only")
	}
	var saved struct {
		Identity clusteridentity.Identity `json:"identity"`
		Pending  string                   `json:"pending,omitempty"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	pending := access.NewToken("kda_")
	// The server committed the replacement, but the process never received its response.
	if _, err := fixture.service.Clusters.Rotate(fixture.ctx, saved.Identity.ID, saved.Identity.Token, pending, "lost-response-process"); err != nil {
		t.Fatal(err)
	}
	saved.Pending = pending
	saved.Identity.ExpiresAt = time.Now().Add(time.Hour)
	data, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := agent.ClientFromFile(fixture.server.URL, path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.RefreshCredential(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	// Unmarshal into a fresh value because omitempty deliberately omits the cleared pending key.
	var committed map[string]json.RawMessage
	if err := json.Unmarshal(data, &committed); err != nil {
		t.Fatal(err)
	}
	if _, exists := committed["pending"]; exists || saved.Identity.Token != pending {
		t.Fatal("pending credential was not committed after recovery")
	}
	if err := fixture.service.Clusters.Revoke(fixture.ctx, principal); err != nil {
		t.Fatal(err)
	}
	err = client.Heartbeat(fixture.ctx)
	var rejected agent.APIError
	if !errors.As(err, &rejected) || rejected.Status != http.StatusForbidden {
		t.Fatal("running client retained revoked authority")
	}
}
