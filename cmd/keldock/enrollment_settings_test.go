package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
)

func TestEnrollmentReadsItsDaemonConfigurationAndPurpose(t *testing.T) {
	for _, test := range []struct {
		name, flag, endpoint string
		configuration        func(agentSettings) any
	}{
		{"workload", "", "/v1/agent/enroll", func(base agentSettings) any { return base }},
		{"database", "--database", "/v1/database/agent/enroll", func(base agentSettings) any {
			return databaseAgentSettings{base, []database.Policy{{Scope: v1.Scope{OrganizationID: "team", ProjectID: "sample"}, ClusterID: "keldock", Limit: 4, Profiles: map[string]database.Profile{"development": {StorageClass: "local-path", StorageSize: "1Gi"}}}}}
		}},
		{"cloud", "--cloud", "/v1/cloud/agent/enroll", func(base agentSettings) any {
			return cloudAgentSettings{APIURL: base.APIURL, CredentialFile: base.CredentialFile, Development: true, Policies: []cloud.Policy{{Scope: v1.Scope{OrganizationID: "team", ProjectID: "sample"}, ClusterID: "keldock", Accounts: []string{"123456789012"}, Regions: []string{"eu-central-1"}}}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests <- request.URL.Path
				if request.Method != http.MethodPost || request.URL.Path != test.endpoint {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				identity := clusteridentity.Identity{ID: "agent-fixture", OrganizationID: "team", ClusterID: "keldock", Generation: 2, Token: strings.Repeat("a", 40), ExpiresAt: time.Now().Add(time.Hour)}
				if err := json.NewEncoder(w).Encode(identity); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			directory := t.TempDir()
			credentialFile := filepath.Join(directory, "identity.json")
			config := filepath.Join(directory, "agent.json")
			base := agentSettings{APIURL: server.URL, CredentialFile: credentialFile, Development: true}
			if err := writePrivateJSON(config, test.configuration(base)); err != nil {
				t.Fatal(err)
			}
			tokenFile := filepath.Join(directory, "enrollment.token")
			if err := os.WriteFile(tokenFile, []byte(strings.Repeat("e", 40)), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"enroll", "--config", config, "--cluster", "keldock", "--token-file", tokenFile}
			if test.flag != "" {
				args = append(args, test.flag)
			}
			if err := identityCommand(context.Background(), args); err != nil {
				t.Fatal(err)
			}
			if endpoint := <-requests; endpoint != test.endpoint {
				t.Fatal("enrollment used another agent purpose")
			}
			info, err := os.Stat(credentialFile)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatal("enrollment did not save a private credential", err)
			}
		})
	}
}

func TestEnrollmentRejectsConflictingPurposeAndUnknownConfiguration(t *testing.T) {
	for _, test := range []struct {
		name          string
		flags         []string
		configuration map[string]any
	}{
		{"conflicting purpose", []string{"--database", "--cloud"}, map[string]any{}},
		{"unknown database field", []string{"--database"}, map[string]any{"apiUrl": "https://example.test", "policies": []any{}, "unsupportedAuthority": true}},
		{"workload cannot ignore policies", nil, map[string]any{"apiUrl": "https://example.test", "policies": []any{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "settings.json")
			if err := writePrivateJSON(path, test.configuration); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"enroll", "--config", path, "--cluster", "keldock", "--token-file", filepath.Join(directory, "not-issued.token")}, test.flags...)
			if err := identityCommand(context.Background(), args); err == nil || strings.Contains(err.Error(), "credential file") {
				t.Fatal("unsupported configuration reached credential exchange", err)
			}
		})
	}
}
