package registryauth_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/registryauth"
)

func TestProjectRepositorySelectionAndPrivateFile(t *testing.T) {
	reader := registryauth.Credential{Repository: "ghcr.io/owner/private", Username: "reader", Password: "a-private-reader-token"}
	other := reader
	other.Repository = "ghcr.io/owner/other"
	config := registryauth.Config{Bundle: &other, Workloads: map[string][]registryauth.Credential{"sample": {reader}, "other": {other}}}
	path := filepath.Join(t.TempDir(), "pull.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := registryauth.Read(path, false)
	if err != nil {
		t.Fatal(err)
	}
	images := map[string]string{"web": reader.Repository + "@sha256:" + strings.Repeat("a", 64)}
	selected, err := loaded.WorkloadCredentials("sample", images)
	if err != nil || len(selected) != 1 {
		t.Fatal("selected private repository missing")
	}
	selected, err = loaded.WorkloadCredentials("other", images)
	if err != nil || len(selected) != 0 {
		t.Fatal("neighboring project received a pull credential")
	}
	docker, err := registryauth.DockerConfig([]registryauth.Credential{reader}, false)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Auths map[string]any `json:"auths"`
	}
	if err := json.Unmarshal(docker, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Auths) != 1 || decoded.Auths[reader.Repository] == nil || decoded.Auths["ghcr.io"] != nil {
		t.Fatal("workload credential widened to every repository on the host")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := registryauth.Read(path, false); err == nil {
		t.Fatal("public pull credential file accepted")
	}
	other.Repository = "registry.example/private"
	if err := (registryauth.Config{Bundle: &other}).Validate(false); err == nil {
		t.Fatal("unvalidated production registry profile advertised")
	}
}
