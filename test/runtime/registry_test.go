//go:build integration

package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/cluster"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/registryauth"
)

func TestPrivateRegistryBundleAndWorkloadCredentialLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	config := configuration(t)
	var credentials struct {
		Publisher, Reader struct{ Username, Password string }
	}
	data, err := os.ReadFile("../../.local/registry-auth/state.json")
	if err != nil {
		t.Fatal("run task local:up first")
	}
	if err := json.Unmarshal(data, &credentials); err != nil {
		t.Fatal(err)
	}
	var images struct {
		PrivateImage string `json:"privateImage"`
	}
	data, err = os.ReadFile("../../.local/fixture-image.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &images); err != nil || images.PrivateImage == "" {
		t.Fatal("build private fixture first")
	}
	input := applicationInput(t)
	input.Definition.Components = map[string]v1.Component{"api": input.Definition.Components["api"]}
	input.Images = map[string]string{"api": images.PrivateImage}
	selection := selectedEnvironment()
	bundle, err := compiler.Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range bundle.Files {
		if bytes.Contains(file, []byte(credentials.Reader.Password)) || bytes.Contains(file, []byte(credentials.Publisher.Password)) {
			t.Fatal("private registry credentials entered compiled OCI content")
		}
	}
	publisher, err := artifacts.NewPublisher(artifacts.RegistryConfig{PushRepository: "localhost:5011/keldock/bundles", PullRepository: "keldock-m1-private-registry:5000/keldock/bundles", Username: credentials.Publisher.Username, Password: credentials.Publisher.Password, PlainHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	reference, err := publisher.Push(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := cluster.New(config.Kubeconfig, config.Context, "keldock-m1-private-registry:5000/keldock/bundles", true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pull.json")
	driver.CredentialsFile = path
	reader := registryauth.Credential{Repository: "keldock-m1-private-registry:5000/keldock/fixture", Username: credentials.Reader.Username, Password: credentials.Reader.Password}
	bundleReader := reader
	bundleReader.Repository = "keldock-m1-private-registry:5000/keldock/bundles"
	private := registryauth.Config{Bundle: &bundleReader, Workloads: map[string][]registryauth.Credential{"neighbor": {reader}}}
	write := func() {
		t.Helper()
		data, err := json.Marshal(private)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 140*time.Second)
		defer cancel()
		cleanEnvironment(t, cleanup, driver, selection)
	})
	private.Bundle.Password = "invalid-reader-credential"
	write()
	if err := driver.Reconcile(ctx, projected(selection, input, reference)); err != nil {
		t.Fatal(err)
	}
	waitSource(t, ctx, selection, "False")
	private.Bundle.Password = credentials.Reader.Password
	write()
	if err := driver.Reconcile(ctx, projected(selection, input, reference)); err != nil {
		t.Fatal(err)
	}
	waitSource(t, ctx, selection, "True")
	waitPullDenied(t, ctx, selection)
	if _, err := kube(t, ctx, "get", "secret", "keldock-image-pull", "-n", selection.Namespace, "-o", "name"); err == nil {
		t.Fatal("neighboring project credential was projected")
	}
	private.Workloads = map[string][]registryauth.Credential{"sample": {reader}}
	private.Workloads["sample"][0].Password = "invalid-reader-credential"
	write()
	if err := driver.Reconcile(ctx, projected(selection, input, reference)); err != nil {
		t.Fatal(err)
	}
	waitPullDenied(t, ctx, selection)
	private.Workloads["sample"][0].Password = credentials.Reader.Password
	write()
	waitCredentialRecovery(t, ctx, driver, selection, input, reference)
	data, err = kube(t, ctx, "get", "secret", "keldock-image-pull", "-n", selection.Namespace, "-o", "json")
	if err != nil {
		t.Fatal("projected pull secret unavailable")
	}
	var secret struct {
		Type string            `json:"type"`
		Data map[string][]byte `json:"data"`
	}
	if err := json.Unmarshal(data, &secret); err != nil {
		t.Fatal("invalid projected pull secret")
	}
	if secret.Type != "kubernetes.io/dockerconfigjson" || len(secret.Data) != 1 || !bytes.Contains(secret.Data[".dockerconfigjson"], []byte(credentials.Reader.Password)) {
		t.Fatal("pull credential rotation did not reach the selected namespace")
	}
	if data, err := kube(t, ctx, "auth", "can-i", "get", "secrets", "-n", selection.Namespace, "--as=system:serviceaccount:"+selection.Namespace+":reconciler"); err == nil || !bytes.Contains(data, []byte("no")) {
		t.Fatal("workload reconciler gained secret-read authority")
	}
	private.Workloads = map[string][]registryauth.Credential{}
	write()
	if err := driver.Reconcile(ctx, projected(selection, input, reference)); err != nil {
		t.Fatal(err)
	}
	if _, err := kube(t, ctx, "get", "secret", "keldock-image-pull", "-n", selection.Namespace, "-o", "name"); err == nil {
		t.Fatal("removed pull profile retained its credential Secret")
	}
	data, err = kube(t, ctx, "get", "serviceaccount", "workload", "-n", selection.Namespace, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var account struct {
		ImagePullSecrets []any `json:"imagePullSecrets"`
	}
	if err := json.Unmarshal(data, &account); err != nil || len(account.ImagePullSecrets) != 0 {
		t.Fatal("removed pull profile retained workload credential references")
	}
}

func waitSource(t *testing.T, ctx context.Context, selection compiler.Selection, state string) {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		data, err := kube(t, ctx, "get", "ocirepository", "keldock-source", "-n", selection.Namespace, "-o", "json")
		if err == nil {
			var source struct {
				Status struct {
					Conditions []struct{ Type, Status, Message string }
				}
			}
			if json.Unmarshal(data, &source) == nil {
				for _, condition := range source.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == state && (state != "False" || authenticationFailure(condition.Message)) {
						return
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("private OCI source did not reach the expected authentication state")
}

func waitPullDenied(t *testing.T, ctx context.Context, selection compiler.Selection) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		data, err := kube(t, ctx, "get", "pods", "-n", selection.Namespace, "-o", "json")
		if err == nil {
			var pods struct {
				Items []struct {
					Status struct {
						ContainerStatuses []struct {
							State struct {
								Waiting *struct{ Reason, Message string }
							}
						}
					}
				}
			}
			if json.Unmarshal(data, &pods) == nil {
				for _, pod := range pods.Items {
					for _, container := range pod.Status.ContainerStatuses {
						if waiting := container.State.Waiting; waiting != nil && (waiting.Reason == "ErrImagePull" || waiting.Reason == "ImagePullBackOff") && authenticationFailure(waiting.Message) {
							return
						}
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("cold private image did not demonstrate denied registry access")
}

func authenticationFailure(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "401") || strings.Contains(message, "403") || strings.Contains(message, "unauthorized") || strings.Contains(message, "no basic auth") || strings.Contains(message, "authorization failed")
}

func waitCredentialRecovery(t *testing.T, ctx context.Context, driver *cluster.Registrar, selection compiler.Selection, input v1.SubmitRelease, reference string) {
	t.Helper()
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if err := driver.Reconcile(ctx, projected(selection, input, reference)); err != nil {
			t.Fatal(err)
		}
		report, err := driver.Observe(ctx, selection, input.Definition, input.Images, reference)
		if err == nil && report.State == "ready" {
			if err := report.Validate(input.Definition); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("rotated registry credentials did not recover selected-revision readiness")
}
