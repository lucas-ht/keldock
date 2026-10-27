//go:build integration

package runtime_test

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
)

func TestAPIAuthorityReceiptsAndObservations(t *testing.T) {
	fixture := newAPI(t)
	input := fixture.sign(t, applicationInput(t))
	for _, token := range []string{"", fixture.api.OperatorToken, fixture.api.AgentToken} {
		fixture.call(t, http.MethodPost, "/v1/releases", map[string]string{"Authorization": "Bearer " + token, "Idempotency-Key": "denied-request"}, input, nil, http.StatusForbidden)
	}
	receipt := fixture.submit(t, input, "accepted-request", http.StatusAccepted)
	if duplicate := fixture.submit(t, input, "accepted-request", http.StatusAccepted); receipt != duplicate {
		t.Fatal("API retry did not recover the original receipt")
	}
	changed := input
	changed.Source.RunID++
	changed = fixture.sign(t, changed)
	fixture.submit(t, changed, "accepted-request", http.StatusConflict)
	changed.ProjectID = "other"
	changed = fixture.sign(t, changed)
	fixture.submit(t, changed, "wrong-project-request", http.StatusForbidden)
	changed = input
	changed.Images = map[string]string{"api": "registry.example/sample:latest"}
	fixture.submit(t, changed, "incomplete-images", http.StatusBadRequest)
	fixture.call(t, http.MethodPost, "/v1/releases", nil, []byte(`{"apiVersion":"keldock.io/v1alpha1","unknown":true}`), nil, http.StatusBadRequest)
	fixture.call(t, http.MethodGet, "/v1/projects/other/operations/"+receipt.OperationID, map[string]string{"Authorization": "Bearer " + fixture.token(t, input.Source)}, nil, nil, http.StatusForbidden)
	fixture.call(t, http.MethodGet, "/v1/agent/desired", map[string]string{"Authorization": "Bearer " + fixture.api.OperatorToken}, nil, nil, http.StatusForbidden)
	fixture.call(t, http.MethodPost, "/v1/github/webhooks", map[string]string{"X-Hub-Signature-256": "sha256=00"}, []byte(`{}`), nil, http.StatusForbidden)
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/cancel", map[string]string{"Authorization": "Bearer " + fixture.token(t, input.Source)}, nil, nil, http.StatusForbidden)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	worker := controlplane.Worker{Store: fixture.store, Publisher: publisher(t), Organization: fixture.scope.OrganizationID}
	if didWork, err := worker.Step(ctx); err != nil || !didWork {
		t.Fatalf("publication: %v %v", didWork, err)
	}
	client, err := agent.NewClient(fixture.server.URL, fixture.api.AgentToken, true)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := client.Desired(ctx)
	if err != nil || len(desired) != 1 {
		t.Fatalf("desired: %d %v", len(desired), err)
	}
	report := v1.Observation{EnvironmentID: receipt.EnvironmentID, DeploymentID: receipt.DeploymentID, Generation: receipt.Generation, Bundle: desired[0].Bundle, State: "reconciling", Reason: "Waiting", ObservedAt: time.Now().UTC()}
	headers := map[string]string{"Authorization": "Bearer " + fixture.api.AgentToken}
	fixture.call(t, http.MethodPost, "/v1/agent/observations", headers, report, nil, http.StatusOK)
	older := report
	older.ObservedAt = older.ObservedAt.Add(-time.Second)
	older.State = "failed"
	fixture.call(t, http.MethodPost, "/v1/agent/observations", headers, older, nil, http.StatusConflict)
	var status operationStatus
	fixture.call(t, http.MethodGet, receipt.StatusURL, map[string]string{"Authorization": "Bearer " + fixture.token(t, input.Source)}, nil, &status, http.StatusOK)
	if status.Stale || status.Report == nil || !reflect.DeepEqual(*status.Report, report) || status.Operation.State != "reconciling" {
		t.Fatalf("status lost current evidence: %+v", status)
	}
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/cancel", map[string]string{"Authorization": "Bearer " + fixture.api.OperatorToken}, nil, nil, http.StatusAccepted)
	report.ObservedAt = time.Now().UTC()
	fixture.call(t, http.MethodPost, "/v1/agent/observations", headers, report, nil, http.StatusConflict)

	// Six independent applications force the real agent client through two pages.
	for index := range 6 {
		input.Definition.Name = "app-" + strconv.Itoa(index)
		fixture.submit(t, fixture.sign(t, input), "paged-request-"+strconv.Itoa(index), http.StatusAccepted)
		if didWork, err := worker.Step(ctx); err != nil || !didWork {
			t.Fatalf("page publication: %v %v", didWork, err)
		}
	}
	desired, err = client.Desired(ctx)
	if err != nil || len(desired) != 7 {
		t.Fatalf("pagination lost environments: %d %v", len(desired), err)
	}
}

func TestCLIServerAndAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	fixture := newAPI(t)
	config := configuration(t)
	directory := t.TempDir()
	binary := filepath.Join(directory, "keldock")
	if output, err := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/keldock").CombinedOutput(); err != nil {
		t.Fatalf("compile CLI: %s %v", output, err)
	}
	privateFile := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	jsonFile := func(name string, value any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return privateFile(name, data)
	}
	agentToken := privateFile("agent-token", []byte(fixture.api.AgentToken))
	settings := map[string]any{
		"databaseUrl": config.DatabaseURL, "listen": strings.TrimPrefix(fixture.server.URL, "http://"), "organizationId": fixture.scope.OrganizationID, "clusterId": "keldock-m1", "domain": "keldock.test",
		"registryPush": config.RegistryPush, "registryPull": config.RegistryPull, "registryPlainHttp": true, "development": true,
		"operatorTokenFile": privateFile("operator-token", []byte(fixture.api.OperatorToken)), "agentTokenFile": agentToken, "webhookSecretFile": privateFile("webhook-secret", fixture.api.WebhookSecret),
		"githubIssuer": fixture.issuer.URL, "githubAudience": "keldock:fixture",
		"projects": []map[string]any{{"id": "sample", "name": "Sample", "repositoryId": 42, "ownerId": 24, "workflowPath": ".github/workflows/build.yaml", "pushRef": "refs/heads/main", "provenanceKeys": map[string]string{"fixture": base64.StdEncoding.EncodeToString(fixture.signing.Public().(ed25519.PublicKey))}}},
	}
	controlConfig := jsonFile("control.json", settings)
	agentConfig := jsonFile("agent.json", map[string]any{"apiUrl": fixture.server.URL, "tokenFile": agentToken, "kubeconfig": config.Kubeconfig, "context": config.Context, "registryPull": config.RegistryPull, "registryPlainHttp": true, "development": true})
	if output, err := exec.CommandContext(ctx, binary, "inspect", "--config", agentConfig).CombinedOutput(); err != nil {
		t.Fatalf("supplied dependency inspection: %s %v", output, err)
	}
	fixture.server.Close() // Reuse the reserved loopback endpoint for the real process.
	startCLI(t, ctx, binary, "serve", "--config", controlConfig, "--migrate")
	healthy := false
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, fixture.server.URL+"/healthz", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := (&http.Client{Timeout: time.Second}).Do(request)
		if err == nil {
			healthy = response.StatusCode == http.StatusOK
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Error(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Error(err)
			}
		}
		if healthy {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !healthy {
		t.Fatal("CLI server did not become healthy")
	}
	input := applicationInput(t)
	encoded, err := x509.MarshalPKCS8PrivateKey(fixture.signing)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := privateFile("signing.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
	inputFile := jsonFile("unsigned.json", input)
	signedFile := filepath.Join(directory, "signed.json")
	if output, err := exec.CommandContext(ctx, binary, "attest", "--private-key", keyPath, "--key-id", "fixture", "--input", inputFile, "--output", signedFile).CombinedOutput(); err != nil {
		t.Fatalf("CI attestation CLI: %s %v", output, err)
	}
	data, err := os.ReadFile(signedFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := v1.DecodeJSON(data, &input); err != nil {
		t.Fatal(err)
	}
	receipt := fixture.submit(t, input, "cli-process-release", http.StatusAccepted)
	driver := registrar(t)
	selection := v1.Placement{EnvironmentID: receipt.EnvironmentID, Generation: receipt.Generation + 1, Namespace: "ke-" + strings.TrimPrefix(receipt.EnvironmentID, "env-")}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 70*time.Second)
		defer stop()
		cleanEnvironment(t, cleanup, driver, selection)
	})
	startCLI(t, ctx, binary, "agent", "--config", agentConfig)
	for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline); {
		status := fixture.status(t, receipt)
		if status.Operation.State == "ready" && !status.Stale && status.Report != nil {
			t.Log("Compiled CLI processes admitted, published, reconciled and reported a real five-component release")
			return
		}
		if status.Operation.State == "failed" {
			t.Fatalf("CLI rollout failed: %+v %+v", status.Operation, status.Report)
		}
		time.Sleep(time.Second)
	}
	t.Fatal("CLI rollout did not finish")
}
