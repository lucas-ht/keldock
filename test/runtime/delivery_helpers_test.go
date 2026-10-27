//go:build integration

package runtime_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/cluster"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func applicationInput(t *testing.T) v1.SubmitRelease {
	t.Helper()
	data, err := os.ReadFile("../../examples/multi-service/keldock.yaml")
	if err != nil {
		t.Fatal(err)
	}
	app, err := v1.DecodeApplication(data)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile("../../.local/fixture-image.json")
	if err != nil {
		t.Fatalf("build fixture first: %v", err)
	}
	var image struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal(data, &image); err != nil {
		t.Fatal(err)
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: "sample", Definition: app,
		Source: v1.Source{RepositoryID: 42, OwnerID: 24, Repository: "example/sample", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main", Event: "push", WorkflowRef: "example/sample/.github/workflows/build.yaml@refs/heads/main", RunID: 1, RunAttempt: 1},
		Images: map[string]string{}, Provenance: json.RawMessage(`{}`)}
	for name := range app.Components {
		input.Images[name] = image.Image
	}
	return input
}

func selectedEnvironment() compiler.Selection {
	id := strings.ToLower(rand.Text())[:16]
	return compiler.Selection{EnvironmentID: "env-" + id, DeploymentID: "dep-" + id, Generation: 1, Namespace: "ke-" + id, Hostname: "preview-" + id + ".keldock.test"}
}

func publisher(t *testing.T) *artifacts.Publisher {
	t.Helper()
	config := configuration(t)
	publisher, err := artifacts.NewPublisher(artifacts.RegistryConfig{PushRepository: config.RegistryPush, PullRepository: config.RegistryPull, PlainHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}

func registrar(t *testing.T) *cluster.Registrar {
	t.Helper()
	config := configuration(t)
	registrar, err := cluster.New(config.Kubeconfig, config.Context, config.RegistryPull, true)
	if err != nil {
		t.Fatal(err)
	}
	return registrar
}

func publishBundle(t *testing.T, ctx context.Context, input v1.SubmitRelease, selection compiler.Selection) string {
	t.Helper()
	bundle, err := compiler.Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := publisher(t).Push(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func kube(t *testing.T, ctx context.Context, args ...string) ([]byte, error) {
	t.Helper()
	config := configuration(t)
	command := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", config.Kubeconfig, "--context", config.Context}, args...)...)
	return command.CombinedOutput()
}

func waitState(t *testing.T, ctx context.Context, registrar *cluster.Registrar, selection compiler.Selection, input v1.SubmitRelease, bundle, state string) v1.Observation {
	t.Helper()
	deadline := time.Now().Add(100 * time.Second)
	var last v1.Observation
	var lastError error
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if err := registrar.Reconcile(ctx, selection, input.Definition, bundle); err != nil {
			lastError = err
			time.Sleep(time.Second)
			continue
		}
		last, lastError = registrar.Observe(ctx, selection, input.Definition, input.Images, bundle)
		if lastError == nil && last.State == state {
			if err := last.Validate(input.Definition); err != nil {
				t.Fatal(err)
			}
			return last
		}
		if lastError == nil && last.State == "failed" && state == "ready" {
			for _, kind := range []string{"pods", "services", "endpointslices", "networkpolicies"} {
				data, err := kube(t, ctx, "get", kind, "-n", selection.Namespace, "-o", "yaml")
				t.Logf("failure diagnostic %s: %s (%v)", kind, data, err)
			}
			for name := range input.Definition.Components {
				data, err := kube(t, ctx, "logs", "-n", selection.Namespace, "-l", "keldock.io/component="+name, "--all-containers=true", "--tail=20")
				t.Logf("component %s logs: %s (%v)", name, data, err)
			}
			t.Fatalf("unexpected rollout failure: %+v", last)
		}
		time.Sleep(time.Second)
	}
	data, err := json.Marshal(last)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("wanted %s; observation=%s error=%v", state, data, lastError)
	return last
}

func route(t *testing.T, ctx context.Context, selection compiler.Selection) map[string]any {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:18080/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = selection.Hostname
	var response *http.Response
	var lastBody string
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		response, err = (&http.Client{Timeout: 4 * time.Second}).Do(request)
		if err == nil && response.StatusCode == http.StatusOK {
			break
		}
		if response != nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 2048))
			if readErr == nil {
				lastBody = string(body)
			}
			if closeErr := response.Body.Close(); closeErr != nil {
				t.Error(closeErr)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil || response == nil {
		t.Fatalf("route unavailable: %v", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		for _, kind := range []string{"ingress", "service", "endpointslice"} {
			data, diagnosticErr := kube(t, ctx, "get", kind, "-n", selection.Namespace, "-o", "yaml")
			t.Logf("route diagnostic %s: %s (%v)", kind, data, diagnosticErr)
		}
		t.Fatalf("route status %d, body %q", response.StatusCode, lastBody)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func cleanEnvironment(t *testing.T, ctx context.Context, registrar *cluster.Registrar, selection compiler.Selection) {
	t.Helper()
	selection.Generation++
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); {
		done, err := registrar.Remove(ctx, selection)
		if err != nil {
			t.Error(fmt.Errorf("environment cleanup: %w", err))
			return
		}
		if done {
			return
		}
		time.Sleep(time.Second)
	}
	t.Error("environment cleanup did not finish")
}
