package compiler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func fixture() (v1.SubmitRelease, Selection) {
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: "sample", Definition: v1.Application{
		APIVersion: v1.APIVersion, Name: "sample", Components: map[string]v1.Component{
			"web":      {Kind: "http", Size: "small", Port: 8080, Probe: &v1.Probe{Path: "/ready"}, Route: true, DependsOn: []string{"migrate"}, ConnectsTo: []string{"api"}},
			"api":      {Kind: "http", Size: "small", Port: 8080, Probe: &v1.Probe{Path: "/ready"}},
			"worker":   {Kind: "worker", Size: "small", Probe: &v1.Probe{Command: []string{"/fixture", "health"}}, ConnectsTo: []string{"api"}},
			"migrate":  {Kind: "job", Size: "small", Command: []string{"/fixture", "job"}},
			"schedule": {Kind: "cronjob", Size: "small", Schedule: "*/5 * * * *"}}},
		Source: v1.Source{RepositoryID: 1, OwnerID: 2, Repository: "example/app", Commit: strings.Repeat("a", 40),
			Ref: "refs/heads/main", Event: "push", WorkflowRef: "example/workflow", RunID: 3, RunAttempt: 1},
		Images: map[string]string{}, Provenance: json.RawMessage(`{}`)}
	for name := range input.Definition.Components {
		input.Images[name] = "registry.example/app@sha256:" + strings.Repeat("a", 64)
	}
	return input, Selection{EnvironmentID: "env-one", DeploymentID: "dep-one", Generation: 1, Namespace: "ke-one", Hostname: "sample.localhost"}
}

func TestCompiledWorkloadBoundaries(t *testing.T) {
	input, selection := fixture()
	bundle, err := Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for name, data := range bundle.Files {
		var object map[string]any
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		kind := object["kind"].(string)
		kinds[kind]++
		if kind == "Kustomization" {
			continue
		}
		metadata := object["metadata"].(map[string]any)
		if metadata["namespace"] != selection.Namespace {
			t.Fatalf("object escaped target namespace: %s", name)
		}
		switch kind {
		case "Deployment", "Job", "CronJob", "Service", "Ingress", "NetworkPolicy":
		default:
			t.Fatalf("unexpected privileged or unsupported kind %s", kind)
		}
	}
	for kind, count := range map[string]int{"Deployment": 3, "Job": 1, "CronJob": 1, "Service": 2, "Ingress": 1, "NetworkPolicy": 5, "Kustomization": 5} {
		if kinds[kind] != count {
			t.Errorf("%s: got %d, want %d", kind, kinds[kind], count)
		}
	}
	policy := networkPolicy("api", input.Definition, selection)
	if len(policy.Spec.Ingress) != 2 || len(policy.Spec.Egress) != 1 {
		t.Fatal("API must allow only named web/worker sources and DNS egress")
	}
	objects := workload("worker", input.Definition.Components["worker"], input.Images["worker"], selection)
	data, err := json.Marshal(objects)
	if err != nil {
		t.Fatal(err)
	}
	for _, requirement := range []string{`"automountServiceAccountToken":false`, `"runAsNonRoot":true`, `"readOnlyRootFilesystem":true`, `"allowPrivilegeEscalation":false`, `"serviceAccountName":"workload"`, `"drop":["ALL"]`} {
		if !bytes.Contains(data, []byte(requirement)) {
			t.Errorf("missing workload restriction %s", requirement)
		}
	}
}

func TestDeterministicArchiveAndJobIdentity(t *testing.T) {
	if len(JobName(strings.Repeat("a", 40), strings.Repeat("b", 40))) > 63 {
		t.Fatal("release job exceeded the Kubernetes name limit")
	}
	input, selection := fixture()
	first, err := Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	firstData, err := first.Archive()
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		second, err := Render(input, selection)
		if err != nil {
			t.Fatal(err)
		}
		data, err := second.Archive()
		if err != nil || !bytes.Equal(firstData, data) {
			t.Fatalf("archive changed for identical input: %v", err)
		}
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(firstData))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			t.Error(err)
		}
	}()
	tarReader := tar.NewReader(gzipReader)
	count := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(header.Name, "components/") || header.Typeflag != tar.TypeReg {
			t.Fatal("unsafe archive member")
		}
		count++
	}
	if count != len(first.Files) {
		t.Fatalf("archive lost files: %d", count)
	}
	selection.DeploymentID = "dep-two"
	second, err := Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first.Files["components/migrate/object-0.json"], second.Files["components/migrate/object-0.json"]) {
		t.Fatal("new deployment reused a previous release job identity")
	}
	first.Files["../escape"] = nil
	if _, err := first.Archive(); err == nil {
		t.Fatal("unsafe archive path accepted")
	}
}
