package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallationUsesRegistryDNSAcrossNamespaces(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "release.json")
	value := releaseManifest{Version: "0.6.0-alpha.1", Revision: strings.Repeat("a", 40), Runtime: "registry.example/runtime@sha256:" + strings.Repeat("a", 64), Console: "registry.example/console@sha256:" + strings.Repeat("b", 64)}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, data, 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "private")
	if err := installationInit(context.Background(), []string{"--directory", directory, "--release-file", manifest, "--name", "control", "--namespace", "team-platform", "--organization", "team", "--public-url", "https://control.example.test", "--domain", "apps.example.test", "--storage-class", "local-path"}); err != nil {
		t.Fatal(err)
	}
	var settings controlSettings
	if err := readSettings(filepath.Join(directory, "control.json"), &settings); err != nil {
		t.Fatal(err)
	}
	expected := "control-registry.team-platform.svc.cluster.local:5000/keldock/bundles"
	if settings.RegistryPush != expected || settings.RegistryPull != expected {
		t.Fatal("registry was only resolvable from the API namespace")
	}
	var agent agentSettings
	if err := readSettings(filepath.Join(directory, "agent.json"), &agent); err != nil {
		t.Fatal(err)
	}
	if agent.RegistryPull != expected {
		t.Fatal("registrar disagrees with the worker's immutable bundle repository")
	}
	for _, name := range []string{"control.json", "agent.json", "installation-secret.json", "webhook.secret", "postgres-password"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("private installation material lost owner-only permissions", name, err)
		}
	}
	if err := installationSecretCommand([]string{"--directory", directory}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateFilesRepairsRetainedVolumeGroupBits(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "input")
	state := filepath.Join(root, "state")
	destination := filepath.Join(root, "runtime")
	for _, path := range []string{source, state} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(state, "identity.json")
	if err := os.WriteFile(path, []byte(`{"synthetic":"credential"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := privateFilesCommand([]string{"--source", source, "--destination", destination, "--state", state}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("replaced pod could not read its retained private identity safely", err)
	}
	if err := os.Symlink(path, filepath.Join(state, "unsafe-link")); err != nil {
		t.Fatal(err)
	}
	if err := privateFilesCommand([]string{"--source", source, "--destination", destination, "--state", state}); err == nil {
		t.Fatal("private-state initialization followed a symlink")
	}
}
