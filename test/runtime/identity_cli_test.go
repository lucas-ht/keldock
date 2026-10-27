//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/agent"
)

func TestCLIAccountRecoveryAndEnrollment(t *testing.T) {
	fixture := newAPI(t)
	ctx := context.Background()
	directory := t.TempDir()
	binary := filepath.Join(directory, "keldock")
	if output, err := exec.Command("go", "build", "-o", binary, "../../cmd/keldock").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %.200s", err, output)
	}
	write := func(name string, value any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	config := configuration(t)
	organization := access.NewID("cli")
	control := write("control.json", map[string]string{"databaseUrl": config.DatabaseURL, "organizationId": organization, "publicUrl": fixture.server.URL})
	outputPath := filepath.Join(directory, "invitation.json")
	output, err := exec.Command(binary, "bootstrap", "--config", control, "--email", "recovery@example.test", "--output", outputPath).CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap: %v %.200s", err, output)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var invitation struct {
		URL string `json:"activationUrl"`
	}
	if err := json.Unmarshal(data, &invitation); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(invitation.URL)
	if err != nil {
		t.Fatal(err)
	}
	values, _ := url.ParseQuery(parsed.Fragment)
	token := values.Get("token")
	if token == "" || strings.Contains(string(output), token) {
		t.Fatal("bootstrap lost or printed the activation credential")
	}
	users, err := access.NewRepository(fixture.store.Pool(), organization)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("initial local recovery test password")
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := users.Activate(ctx, token, hash)
	if err != nil {
		t.Fatal(err)
	}
	passwordPath := filepath.Join(directory, "replacement-password")
	replacement := "replacement local recovery test password"
	if err := os.WriteFile(passwordPath, []byte(replacement), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err = exec.Command(binary, "recover", "--config", control, "--email", "recovery@example.test", "--password-file", passwordPath).CombinedOutput()
	if err != nil || strings.Contains(string(output), replacement) {
		t.Fatal("CLI recovery failed or printed credential material")
	}
	if _, _, err := users.Session(ctx, session); err == nil {
		t.Fatal("CLI recovery retained the previous session")
	}
	account, err := users.PasswordAccount(ctx, "recovery@example.test")
	if err != nil || !access.VerifyPassword(account.Hash, replacement) {
		t.Fatal("CLI recovery did not set the supplied password")
	}
	principal, _, err := fixture.api.Security.Users.Session(ctx, fixture.session)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := fixture.api.Security.Clusters.CreateEnrollment(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentPath := filepath.Join(directory, "enrollment.token")
	if err := os.WriteFile(enrollmentPath, []byte(enrollment.Token), 0o600); err != nil {
		t.Fatal(err)
	}
	credentialPath := filepath.Join(directory, "agent-credential.json")
	agentConfig := write("agent.json", map[string]any{"apiUrl": fixture.server.URL, "credentialFile": credentialPath, "development": true})
	output, err = exec.Command(binary, "enroll", "--config", agentConfig, "--cluster", "keldock-m1", "--token-file", enrollmentPath).CombinedOutput()
	if err != nil || strings.Contains(string(output), enrollment.Token) {
		t.Fatal("CLI enrollment failed or printed the grant")
	}
	data, err = os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Identity clusteridentity.Identity `json:"identity"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), state.Identity.Token) {
		t.Fatal("CLI printed its enrolled credential")
	}
	client, err := agent.ClientFromFile(fixture.server.URL, credentialPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
