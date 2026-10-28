// Package recovery creates private, independently restorable installation snapshots.
package recovery

import (
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/registryauth"
)

// Config is supplied by the installation operator, never a public API request.
type Config struct {
	Organization string
	DatabaseURL  string
	Files        map[string]string
	Registry     artifacts.RegistryConfig
	Pulls        registryauth.Config
}

// Artifact records an immutable OCI graph and the tag used inside the snapshot layout.
type Artifact struct {
	Reference string `json:"reference"`
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
	Tag       string `json:"tag"`
}

// Manifest is a versioned checksum inventory; it contains no plaintext credentials.
type Manifest struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Organization string            `json:"organizationId"`
	CreatedAt    time.Time         `json:"createdAt"`
	Files        map[string]string `json:"sha256"`
	Artifacts    []Artifact        `json:"artifacts"`
}
