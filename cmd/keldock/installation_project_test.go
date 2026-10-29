package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingProjectCanExplicitlyEnablePostgreSQL(t *testing.T) {
	directory, policyFile, project := installedProject(t)
	beforeCore := installationFiles(t, directory)
	if err := installationProject(context.Background(), []string{"--directory", directory, "--file", policyFile}); err == nil {
		t.Fatal("an existing project was rewritten without explicit delegation")
	}
	assertInstallationFiles(t, directory, beforeCore)
	args := []string{"--directory", directory, "--file", policyFile, "--postgresql"}
	if err := installationProject(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	var settings controlSettings
	if err := readSettings(filepath.Join(directory, "control.json"), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Projects) != 1 || len(settings.Databases) != 1 || settings.Databases[0].ProjectID != project.ID || settings.Databases[0].ClusterID != settings.Cluster {
		t.Fatal("optional delegation duplicated or changed project placement")
	}
	encoded, err := json.Marshal(settings.Projects[0])
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(project)
	if err != nil || string(encoded) != string(expected) {
		t.Fatal("database delegation changed repository authority", err)
	}
	var databaseAgent struct {
		agentSettings
		Policies []json.RawMessage `json:"policies"`
	}
	if err := readSettings(filepath.Join(directory, "database-agent.json"), &databaseAgent); err != nil || len(databaseAgent.Policies) != 1 {
		t.Fatal("independent provisioner configuration missing", err)
	}
	var values struct {
		Databases struct{ Enabled bool } `json:"databases"`
	}
	data, err := os.ReadFile(filepath.Join(directory, "values.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &values); err != nil || !values.Databases.Enabled {
		t.Fatal("explicit database process was not enabled", err)
	}
	before := installationFiles(t, directory)
	if err := installationProject(context.Background(), args); err == nil {
		t.Fatal("repeated delegation could overwrite an existing database policy")
	}
	assertInstallationFiles(t, directory, before)
}

func TestExistingProjectDelegationCannotChangeRepositoryAuthority(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*repositorySettings)
	}{
		{"owner", func(p *repositorySettings) { p.OwnerID++ }},
		{"repository", func(p *repositorySettings) { p.RepositoryID++ }},
		{"project", func(p *repositorySettings) { p.ID = "different" }},
		{"workflow", func(p *repositorySettings) { p.WorkflowPath = ".github/workflows/other.yaml" }},
		{"signing key", func(p *repositorySettings) { p.ProvenanceKeys["other"] = p.ProvenanceKeys["build"] }},
		{"preview authority", func(p *repositorySettings) { p.Previews.MaxActive = 20 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory, policyFile, project := installedProject(t)
			test.change(&project)
			if err := writePrivateJSON(policyFile, project); err != nil {
				t.Fatal(err)
			}
			before := installationFiles(t, directory)
			if err := installationProject(context.Background(), []string{"--directory", directory, "--file", policyFile, "--postgresql"}); err == nil {
				t.Fatal("optional capability changed repository authority")
			}
			assertInstallationFiles(t, directory, before)
		})
	}
}

func installedProject(t *testing.T) (string, string, repositorySettings) {
	t.Helper()
	root := t.TempDir()
	manifest := filepath.Join(root, "release.json")
	if err := writePrivateJSON(manifest, releaseManifest{Version: "0.6.0-alpha.1", Revision: strings.Repeat("a", 40), Runtime: "registry.example/runtime@sha256:" + strings.Repeat("a", 64), Console: "registry.example/console@sha256:" + strings.Repeat("b", 64)}); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "private")
	if err := installationInit(context.Background(), []string{"--directory", directory, "--release-file", manifest, "--organization", "team", "--public-url", "https://control.example.test", "--domain", "apps.example.test", "--storage-class", "local-path"}); err != nil {
		t.Fatal(err)
	}
	project := repositorySettings{ID: "sample", Name: "Sample", RepositoryID: 123, OwnerID: 456, WorkflowPath: ".github/workflows/deploy.yaml", PushRef: "refs/heads/main", ProvenanceKeys: map[string]string{"build": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))}}
	policyFile := filepath.Join(root, "repository-policy.json")
	if err := writePrivateJSON(policyFile, project); err != nil {
		t.Fatal(err)
	}
	if err := installationProject(context.Background(), []string{"--directory", directory, "--file", policyFile}); err != nil {
		t.Fatal(err)
	}
	return directory, policyFile, project
}

func installationFiles(t *testing.T, directory string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(data)
	}
	return files
}

func assertInstallationFiles(t *testing.T, directory string, expected map[string]string) {
	t.Helper()
	actual := installationFiles(t, directory)
	if len(actual) != len(expected) {
		t.Fatal("rejected delegation changed private installation members")
	}
	for name, before := range expected {
		if actual[name] != before {
			t.Fatal("rejected delegation rewrote private configuration", name)
		}
	}
}
