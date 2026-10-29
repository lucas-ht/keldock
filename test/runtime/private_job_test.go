//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/registryauth"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// A new source can reach Flux before the registrar updates its generation-specific
// pull patch. Model that ordering deterministically by creating the exact new Job
// without a template pull reference, then reconciling the late patch through Flux.
func TestPrivateReleaseJobSurvivesLatePullPatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var credentials struct {
		Reader struct{ Username, Password string }
	}
	data, err := os.ReadFile("../../.local/registry-auth/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &credentials); err != nil {
		t.Fatal(err)
	}
	var images struct{ PrivateImage string }
	data, err = os.ReadFile("../../.local/fixture-image.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &images); err != nil || images.PrivateImage == "" {
		t.Fatal("private fixture image required", err)
	}
	repository, _, err := artifacts.SplitReference(images.PrivateImage)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pull.json")
	profile := registryauth.Config{Workloads: map[string][]registryauth.Credential{"sample": {{Repository: repository, Username: credentials.Reader.Username, Password: credentials.Reader.Password}}}}
	data, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	driver := registrar(t)
	driver.CredentialsFile = path
	selection := selectedEnvironment()
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 140*time.Second)
		defer stop()
		cleanEnvironment(t, cleanup, driver, selection)
	})
	input := applicationInput(t)
	migration := input.Definition.Components["migrate"]
	input.Definition.Components = map[string]v1.Component{"api": input.Definition.Components["api"]}
	input.Images = map[string]string{"api": images.PrivateImage}
	first := publishBundle(t, ctx, input, selection)
	waitState(t, ctx, driver, selection, input, first, "ready")

	selection.Generation++
	selection.DeploymentID += "-updated"
	input.Definition.Components["migrate"] = migration
	input.Images["migrate"] = images.PrivateImage
	bundle, err := compiler.Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	jobFile := filepath.Join(t.TempDir(), "job.json")
	if err := os.WriteFile(jobFile, bundle.Files["components/migrate/object-0.json"], 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := kube(t, ctx, "create", "-f", jobFile); err != nil {
		t.Fatalf("create early selected Job: %s (%v)", output, err)
	}
	before := releaseJob(t, ctx, selection)
	if len(before.Spec.Template.Spec.ImagePullSecrets) != 0 {
		t.Fatal("regression setup already had the late template patch")
	}
	second := publishBundle(t, ctx, input, selection)
	waitState(t, ctx, driver, selection, input, second, "ready")
	after := releaseJob(t, ctx, selection)
	if after.UID != before.UID || after.Status.Succeeded != 1 || len(after.Spec.Template.Spec.ImagePullSecrets) != 0 {
		t.Fatal("late pull patch changed or recreated the completed selected Job")
	}
	data, err = kube(t, ctx, "get", "pods", "-n", selection.Namespace, "-l", "job-name="+after.Name, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var pods corev1.PodList
	if err := json.Unmarshal(data, &pods); err != nil || len(pods.Items) != 1 {
		t.Fatal("selected Job Pod evidence unavailable", err)
	}
	refs := pods.Items[0].Spec.ImagePullSecrets
	if len(refs) != 1 || refs[0].Name != "keldock-image-pull" || pods.Items[0].Spec.ServiceAccountName != "workload" {
		t.Fatal("Job Pod did not inherit its operator-owned ServiceAccount reader")
	}
}

func releaseJob(t *testing.T, ctx context.Context, selection compiler.Selection) batchv1.Job {
	t.Helper()
	data, err := kube(t, ctx, "get", "job", compiler.JobName("migrate", selection.DeploymentID), "-n", selection.Namespace, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var job batchv1.Job
	if err := json.Unmarshal(data, &job); err != nil {
		t.Fatal(err)
	}
	return job
}
