//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/agent"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
)

func (fixture *apiFixture) status(t *testing.T, receipt v1.Receipt) operationStatus {
	t.Helper()
	var status operationStatus
	fixture.call(t, http.MethodGet, receipt.StatusURL, map[string]string{"Authorization": "Bearer " + fixture.api.OperatorToken}, nil, &status, http.StatusOK)
	return status
}

func waitAgent(t *testing.T, ctx context.Context, fixture *apiFixture, runtime agent.Agent, receipt v1.Receipt, state string) operationStatus {
	t.Helper()
	var last operationStatus
	var lastError error
	for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline) && ctx.Err() == nil; {
		lastError = runtime.Step(ctx)
		last = fixture.status(t, receipt)
		if lastError == nil && last.Operation.State == state {
			return last
		}
		if state == "ready" && last.Operation.State == "failed" {
			t.Fatalf("unexpected API rollout failure: %+v report=%+v error=%v", last.Operation, last.Report, lastError)
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("wanted API state %s, got %+v report=%+v error=%v", state, last.Operation, last.Report, lastError)
	return last
}

func previewInput(t *testing.T, number int64) v1.SubmitRelease {
	t.Helper()
	input := applicationInput(t)
	input.Source.Event = "pull_request"
	input.Source.PullRequest = number
	input.Source.HeadCommit = strings.Repeat("b", 40)
	input.Source.Ref = "refs/pull/" + strconv.FormatInt(number, 10) + "/merge"
	input.Source.WorkflowRef = "example/sample/.github/workflows/build.yaml@" + input.Source.Ref
	return input
}

func TestAPIWorkerAgentPreviewLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	fixture := newAPI(t)
	client, err := agent.NewClient(fixture.server.URL, fixture.api.AgentToken, true)
	if err != nil {
		t.Fatal(err)
	}
	driver := registrar(t)
	runtime := agent.Agent{Client: client, Driver: driver}
	worker := controlplane.Worker{Store: fixture.store, Publisher: publisher(t), Organization: fixture.scope.OrganizationID}
	publish := func() {
		t.Helper()
		if worked, err := worker.Step(ctx); err != nil || !worked {
			t.Fatalf("durable worker: %v %v", worked, err)
		}
	}
	input := previewInput(t, 11)
	opened := time.Now().UTC()
	fixture.webhook(t, input, "opened", opened)
	input = fixture.sign(t, input)
	first := fixture.submit(t, input, "preview-initial", http.StatusAccepted)
	if duplicate := fixture.submit(t, input, "preview-initial", http.StatusAccepted); first != duplicate {
		t.Fatal("lost-client retry changed receipt")
	}
	publish()
	desired, err := client.Current(ctx, first.EnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	selection := desired.Selection
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 70*time.Second)
		defer stop()
		selection.Generation += 10
		cleanEnvironment(t, cleanup, driver, selection)
	})
	ready := waitAgent(t, ctx, fixture, runtime, first, "ready")
	if ready.Stale || ready.Report == nil || len(ready.Report.Components) != 5 {
		t.Fatal("API did not receive complete fresh health")
	}
	route(t, ctx, selection)
	webPods, err := kube(t, ctx, "get", "pods", "-n", selection.Namespace, "-l", "keldock.io/component=web", "-o", "jsonpath={.items[0].metadata.uid}")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../.local/fixture-image.json")
	if err != nil {
		t.Fatal(err)
	}
	var images struct {
		Image        string `json:"image"`
		UpdatedImage string `json:"updatedImage"`
	}
	if err := json.Unmarshal(data, &images); err != nil || images.UpdatedImage == "" || images.UpdatedImage == images.Image {
		t.Fatal("distinct synthetic CI image required")
	}
	input.Images["api"] = images.UpdatedImage
	input.Source.RunID++
	input = fixture.sign(t, input)
	second := fixture.submit(t, input, "preview-api-update", http.StatusAccepted)
	publish()
	// A restarted agent has no local journal; PostgreSQL and cluster evidence drive recovery.
	restartedClient, err := agent.NewClient(fixture.server.URL, fixture.api.AgentToken, true)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Client = restartedClient
	if second.EnvironmentID != first.EnvironmentID || second.Generation != first.Generation+1 {
		t.Fatal("PR update did not select the stable preview with a new generation")
	}
	updated := waitAgent(t, ctx, fixture, runtime, second, "ready")
	if updated.Report.DeploymentID != second.DeploymentID {
		t.Fatal("old evidence satisfied the updated release")
	}
	response := route(t, ctx, selection)
	api, ok := response["api"].(map[string]any)
	if !ok || api["artifact"] != "v2" {
		t.Fatalf("API did not run the updated immutable image: %+v", response)
	}
	after, err := kube(t, ctx, "get", "pods", "-n", selection.Namespace, "-l", "keldock.io/component=web", "-o", "jsonpath={.items[0].metadata.uid}")
	if err != nil || string(webPods) != string(after) {
		t.Fatalf("unchanged web image caused an unnecessary pod replacement: %v", err)
	}
	input.Definition.Components["migrate"].Env["FAIL_JOB"] = "true"
	input.Source.RunID++
	failedJob := fixture.submit(t, fixture.sign(t, input), "failed-prerequisite", http.StatusAccepted)
	publish()
	failed := waitAgent(t, ctx, fixture, runtime, failedJob, "failed")
	for _, component := range failed.Report.Components {
		if component.Name == "web" && component.Ready {
			t.Fatal("failed prerequisite permitted selected-release readiness")
		}
	}
	delete(input.Definition.Components["migrate"].Env, "FAIL_JOB")
	input.Source.RunID++
	recovered := fixture.submit(t, fixture.sign(t, input), "recovered-prerequisite", http.StatusAccepted)
	publish()
	waitAgent(t, ctx, fixture, runtime, recovered, "ready")
	otherInput := previewInput(t, 12)
	fixture.webhook(t, otherInput, "opened", opened)
	other := fixture.submit(t, fixture.sign(t, otherInput), "other-preview", http.StatusAccepted)
	publish()
	otherDesired, err := client.Current(ctx, other.EnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 70*time.Second)
		defer stop()
		cleanEnvironment(t, cleanup, driver, otherDesired.Selection)
	})
	waitAgent(t, ctx, fixture, runtime, other, "ready")
	if data, err := kube(t, ctx, "exec", "-n", selection.Namespace, "deployment/web", "--", "/fixture", "probe", "http://api."+otherDesired.Selection.Namespace+".svc.cluster.local:8080/ready"); err == nil {
		t.Fatalf("API-admitted previews were not isolated: %s", data)
	}
	fixture.webhook(t, input, "closed", opened.Add(time.Second))
	fixture.webhook(t, input, "opened", opened) // Out-of-order open must not resurrect it.
	input.Source.RunID++
	fixture.submit(t, fixture.sign(t, input), "late-ci-preview", http.StatusConflict)
	deletion, err := client.Current(ctx, first.EnvironmentID)
	if err != nil || deletion.Operation.Kind != "delete" {
		t.Fatalf("close event did not persist teardown: %+v %v", deletion, err)
	}
	deleted := v1.Receipt{OperationID: deletion.Operation.ID, StatusURL: "/v1/projects/sample/operations/" + deletion.Operation.ID}
	waitAgent(t, ctx, fixture, runtime, deleted, "deleted")
	if _, err := kube(t, ctx, "get", "namespace", selection.Namespace); err == nil {
		t.Fatal("closed preview namespace remains")
	}
	route(t, ctx, otherDesired.Selection)
	t.Log("Verified signed PR events -> OIDC/provenance admission -> durable worker -> OCI -> outbound agent -> Flux -> revision-bound API health; API-only update and preview teardown")
}
