package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestPrepareRequiresCompleteImagesAndTrustedGitHubContext(t *testing.T) {
	directory := t.TempDir()
	for name, value := range map[string]string{"GITHUB_REPOSITORY_ID": "42", "GITHUB_REPOSITORY_OWNER_ID": "24", "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1", "GITHUB_REPOSITORY": "owner/sample", "GITHUB_SHA": strings.Repeat("a", 40), "GITHUB_REF": "refs/heads/main", "GITHUB_EVENT_NAME": "push", "GITHUB_WORKFLOW_REF": "owner/sample/.github/workflows/deploy.yml@refs/heads/main"} {
		t.Setenv(name, value)
	}
	definition := filepath.Join(directory, "keldock.yaml")
	images := filepath.Join(directory, "images.json")
	output := filepath.Join(directory, "unsigned.json")
	if err := os.WriteFile(definition, []byte("apiVersion: keldock.io/v1alpha1\nname: sample\ncomponents:\n  web:\n    kind: http\n    size: small\n    port: 8080\n    probe: { path: /ready }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"web": "ghcr.io/owner/sample@sha256:" + strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(images, data, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--file", definition, "--images", images, "--project", "sample", "--output", output}
	if err := prepareCommand(args); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var release v1.SubmitRelease
	if err := v1.DecodeJSON(data, &release); err != nil {
		t.Fatal(err)
	}
	if release.Source.RepositoryID != 42 || len(release.Images) != 1 || string(release.Provenance) != "{}" {
		t.Fatal("preparation changed input authority or pretended to sign")
	}
	if err := prepareCommand(args); err == nil {
		t.Fatal("existing release output overwritten")
	}
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_REF", "refs/pull/1/merge")
	event := filepath.Join(directory, "event.json")
	if err := os.WriteFile(event, []byte(`{"number":1,"pull_request":{"head":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repo":{"id":99}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_EVENT_PATH", event)
	if _, err := githubSource(); err == nil {
		t.Fatal("fork source admitted to the signing profile")
	}
}
