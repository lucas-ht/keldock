package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/lucas-ht/keldock/internal/admission"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
)

type repositorySettings struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	RepositoryID   int64             `json:"repositoryId"`
	OwnerID        int64             `json:"ownerId"`
	WorkflowPath   string            `json:"workflowPath"`
	PushRef        string            `json:"pushRef"`
	ProvenanceKeys map[string]string `json:"provenanceKeys"`
	Previews       v1.PreviewPolicy  `json:"previews"`
}

type controlSettings struct {
	DatabaseURL           string               `json:"databaseUrl"`
	Listen                string               `json:"listen"`
	Organization          string               `json:"organizationId"`
	Cluster               string               `json:"clusterId"`
	Domain                string               `json:"domain"`
	RegistryPush          string               `json:"registryPush"`
	RegistryPull          string               `json:"registryPull"`
	RegistryUsername      string               `json:"registryUsername,omitempty"`
	RegistryPasswordFile  string               `json:"registryPasswordFile,omitempty"`
	RegistryPlainHTTP     bool                 `json:"registryPlainHttp"`
	PublicURL             string               `json:"publicUrl"`
	WebhookSecretFile     string               `json:"webhookSecretFile"`
	GitHubIssuer          string               `json:"githubIssuer"`
	GitHubAudience        string               `json:"githubAudience"`
	GitHubCredentialsFile string               `json:"githubCredentialsFile,omitempty"`
	Development           bool                 `json:"development"`
	Projects              []repositorySettings `json:"projects"`
	Cloud                 []cloud.Policy       `json:"cloud,omitempty"`
	Databases             []database.Policy    `json:"databases,omitempty"`
}

type agentSettings struct {
	APIURL                  string `json:"apiUrl"`
	CredentialFile          string `json:"credentialFile"`
	RegistryCredentialsFile string `json:"registryCredentialsFile,omitempty"`
	Kubeconfig              string `json:"kubeconfig,omitempty"`
	Context                 string `json:"context,omitempty"`
	RegistryPull            string `json:"registryPull"`
	RegistryPlainHTTP       bool   `json:"registryPlainHttp"`
	Development             bool   `json:"development"`
}

func readSettings(path string, target any) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("configuration requires owner-only file permissions")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return v1.DecodeJSON(data, target)
}

func secret(path string) (string, error) {
	return credential(path, 32)
}

func credential(path string, minimum int) (string, error) {
	if path == "" {
		return "", fmt.Errorf("credential file required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read credential file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 4096 {
		return "", fmt.Errorf("credential file requires owner-only permissions")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if len(value) < minimum || len(value) > 4096 {
		return "", fmt.Errorf("credential requires %d–4096 bytes", minimum)
	}
	return value, nil
}

func (settings controlSettings) policy(project repositorySettings) (admission.RepositoryPolicy, error) {
	keys := map[string]ed25519.PublicKey{}
	for id, value := range project.ProvenanceKeys {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(decoded) != ed25519.PublicKeySize {
			return admission.RepositoryPolicy{}, fmt.Errorf("invalid configured provenance key")
		}
		keys[id] = ed25519.PublicKey(decoded)
	}
	return admission.RepositoryPolicy{Scope: v1.Scope{OrganizationID: settings.Organization, ProjectID: project.ID}, RepositoryID: project.RepositoryID, OwnerID: project.OwnerID, WorkflowPath: project.WorkflowPath, PushRef: project.PushRef, Keys: keys}, nil
}
