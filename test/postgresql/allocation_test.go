//go:build integration

package postgresql_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func TestRealPostgreSQLAllocationAndIsolatedPreview(t *testing.T) {
	f := setup(t)
	definition, err := os.ReadFile("../../examples/postgresql/keldock.yaml")
	if err != nil {
		t.Fatal(err)
	}
	app, err := v1.DecodeApplication(definition)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../.local/m6/application-image.json")
	if err != nil {
		t.Fatal("build alpha acceptance application")
	}
	var image struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal(data, &image); err != nil {
		t.Fatal(err)
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: f.scope.ProjectID, Definition: app, Images: map[string]string{}, Source: v1.Source{RepositoryID: 42, OwnerID: 24, Repository: "example/notebook", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main", Event: "push", WorkflowRef: "example/notebook/.github/workflows/build.yaml@refs/heads/main", RunID: 1, RunAttempt: 1}, Provenance: json.RawMessage(`{}`)}
	for name := range app.Components {
		input.Images[name] = image.Image
	}
	staging, err := f.store.AdmitToDomain(f.ctx, f.scope, "synthetic-local-fixture", "staging-postgresql", "keldock.test", input)
	if err != nil {
		t.Fatal(err)
	}
	first := ready(t, f, staging)
	response := request(t, first.Selection.Hostname, "POST", "/markers/staging-canary")
	if response.StatusCode != http.StatusCreated {
		t.Fatal("staging marker write failed")
	}
	closeResponse(t, response)
	response = request(t, first.Selection.Hostname, "GET", "/")
	body, err := io.ReadAll(response.Body)
	closeResponse(t, response)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"staging-canary", `"superuser":false`, `"createDatabase":false`, `"createRole":false`} {
		if !strings.Contains(string(body), text) {
			t.Fatalf("database runtime missing %s", text)
		}
	}
	stagingDatabase := first.Selection.Databases["data"]
	probe := kube(t, f, "exec", "-n", first.Selection.Namespace, "deployment/web", "--", "/app", "probe", stagingDatabase.Host)
	if probe == nil {
		t.Fatal("unbound web component reached PostgreSQL")
	}
	input.Source.Event = "pull_request"
	input.Source.PullRequest = 6
	input.Source.Ref = "refs/pull/6/merge"
	input.Source.HeadCommit = input.Source.Commit
	update := postgres.PullRequestUpdate{RepositoryID: 42, Number: 6, HeadCommit: input.Source.HeadCommit, HeadRepositoryID: 42, UpdatedAt: time.Now().UTC()}
	if _, err := f.store.RecordPullRequest(f.ctx, f.scope, f.scope.OrganizationID+"-open", "open", update); err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.AdmitToDomain(f.ctx, f.scope, "synthetic-local-fixture", "preview-postgresql", "keldock.test", input)
	if err != nil {
		t.Fatal(err)
	}
	second := ready(t, f, preview)
	if second.Selection.Databases["data"].Namespace == stagingDatabase.Namespace {
		t.Fatal("preview used retained staging data")
	}
	response = request(t, second.Selection.Hostname, "GET", "/")
	body, err = io.ReadAll(response.Body)
	closeResponse(t, response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "staging-canary") {
		t.Fatal("new preview inherited staging data")
	}
	if err := kube(t, f, "exec", "-n", second.Selection.Namespace, "deployment/api", "--", "/app", "probe", stagingDatabase.Host); err == nil {
		t.Fatal("preview reached staging database")
	}
	update.Closed = true
	update.UpdatedAt = update.UpdatedAt.Add(time.Second)
	if _, err := f.store.RecordPullRequest(f.ctx, f.scope, f.scope.OrganizationID+"-close", "close", update); err != nil {
		t.Fatal(err)
	}
	deleted := false
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); {
		if err := f.provisioner.Step(f.ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.registrar.Step(f.ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		if err := f.store.Pool().QueryRow(f.ctx, `SELECT state='deleted' FROM operations WHERE environment_id=$1 ORDER BY generation DESC LIMIT 1`, preview.EnvironmentID).Scan(&deleted); err != nil {
			t.Fatal(err)
		}
		if deleted {
			break
		}
	}
	if !deleted {
		t.Fatal("preview cleanup never reached the deleted observation")
	}
	if err := kube(t, f, "get", "namespace", second.Selection.Namespace); err == nil {
		t.Fatal("closed preview workload namespace survived cleanup")
	}
	if err := kube(t, f, "get", "cluster.postgresql.cnpg.io", "database", "-n", second.Selection.Databases["data"].Namespace); err != nil {
		t.Fatal("preview deletion removed retained database")
	}
	response = request(t, first.Selection.Hostname, "GET", "/")
	body, err = io.ReadAll(response.Body)
	closeResponse(t, response)
	if err != nil || !strings.Contains(string(body), "staging-canary") {
		t.Fatal("preview cleanup affected staging data")
	}
	backupAndRestore(t, f, first)
	controlPlaneRecovery(t, f, first, input)
	t.Logf("Retained synthetic data for subsequent recovery acceptance: organization=%s staging=%s database=%s preview-database=%s", f.scope.OrganizationID, staging.EnvironmentID, stagingDatabase.Namespace, second.Selection.Databases["data"].Namespace)
}

func ready(t *testing.T, f *fixture, receipt v1.Receipt) *v1.ProjectedDeployment {
	t.Helper()
	var last string
	for deadline := time.Now().Add(4 * time.Minute); time.Now().Before(deadline); {
		if _, err := f.worker.Step(f.ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.provisioner.Step(f.ctx); err != nil {
			last = err.Error()
		}
		if err := f.registrar.Step(f.ctx); err != nil {
			last = err.Error()
		}
		operation, err := f.store.Operation(f.ctx, f.scope, receipt.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if operation.State == "ready" {
			current, err := f.store.CurrentDesired(f.ctx, f.scope.OrganizationID, "keldock-m6", receipt.EnvironmentID)
			if err != nil {
				t.Fatal(err)
			}
			return current
		}
		if operation.State == "failed" {
			var report []byte
			if err := f.store.Pool().QueryRow(f.ctx, `SELECT report FROM observations WHERE environment_id=$1`, receipt.EnvironmentID).Scan(&report); err == nil {
				t.Logf("failed observation: %s", report)
			}
			t.Fatalf("application failed: %s", operation.Error)
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("PostgreSQL application did not become ready: %s", last)
	return nil
}

func request(t *testing.T, hostname, method, path string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, "http://127.0.0.1:28080"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = hostname
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func closeResponse(t *testing.T, response *http.Response) {
	t.Helper()
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func kube(t *testing.T, f *fixture, args ...string) error {
	t.Helper()
	command := exec.CommandContext(f.ctx, "kubectl", append([]string{"--kubeconfig", f.kubeconfig, "--context", "k3d-keldock-m6"}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fixture command: %s: %w", output, err)
	}
	return nil
}
