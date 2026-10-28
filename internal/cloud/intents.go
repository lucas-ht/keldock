package cloud

import (
	"encoding/json"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Intent is durable desired state, independently retained from deployment namespaces.
// Exactly one typed payload corresponds to Kind.
type Intent struct {
	v1.Scope
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Generation  int64          `json:"generation"`
	Revision    int64          `json:"revision"`
	Action      string         `json:"action"`
	State       string         `json:"state"`
	Connection  *Connection    `json:"connection,omitempty"`
	Resource    *Resource      `json:"resource,omitempty"`
	Binding     *Binding       `json:"binding,omitempty"`
	Domain      *DomainClaim   `json:"domain,omitempty"`
	Backup      *BackupBinding `json:"backup,omitempty"`
	Observation *Evidence      `json:"observation,omitempty"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// Evidence reports identity, external effects and controller state separately.
type Evidence struct {
	ProjectID            string            `json:"projectId"`
	ID                   string            `json:"id"`
	Generation           int64             `json:"generation"`
	ConnectionGeneration int64             `json:"connectionGeneration"`
	State                string            `json:"state"`
	Reason               string            `json:"reason"`
	Details              map[string]string `json:"details,omitempty"`
	ObservedAt           time.Time         `json:"observedAt"`
}

// Mutation carries optimistic concurrency and a receipt identity for lifecycle actions.
type Mutation struct {
	Generation        int64  `json:"generation"`
	Action            string `json:"action"`
	ReviewHash        string `json:"reviewHash,omitempty"`
	ArchiveGeneration int64  `json:"archiveGeneration,omitempty"`
}

// Receipt identifies an accepted cloud revision, not successful external effects.
type Receipt struct {
	APIVersion string `json:"apiVersion"`
	ID         string `json:"id"`
	Generation int64  `json:"generation"`
	StatusURL  string `json:"statusUrl"`
}

// Command is persisted only through typed, server-validated handlers.
type Command struct {
	Kind  string          `json:"kind"`
	ID    string          `json:"id,omitempty"`
	Input json.RawMessage `json:"input"`
}

// DatabaseTarget is an operator-supplied CNPG cluster with explicitly delegated backup fields.
type DatabaseTarget struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	UID              string `json:"uid"`
	RestoreNamespace string `json:"restoreNamespace"`
	Image            string `json:"image"`
	StorageClass     string `json:"storageClass"`
	StorageSize      string `json:"storageSize"`
}

// BackupRequest binds a retained store to an approved existing database.
type BackupRequest struct {
	ResourceID string `json:"resourceId"`
	DatabaseID string `json:"databaseId"`
	Schedule   string `json:"schedule"`
}

// BackupBinding keeps source identity, archive generation and restore authority explicit.
type BackupBinding struct {
	ResourceID        string              `json:"resourceId"`
	DatabaseID        string              `json:"databaseId"`
	Database          DatabaseTarget      `json:"database"`
	ArchiveGeneration int64               `json:"archiveGeneration"`
	ArchivePrefix     string              `json:"archivePrefix"`
	WriterRoleARN     string              `json:"writerRoleArn"`
	RestoreRoleARN    string              `json:"restoreRoleArn"`
	Schedule          string              `json:"schedule"`
	Archives          []ArchiveGeneration `json:"archives"`
	RestoreName       string              `json:"restoreName"`
	RestoreGeneration int64               `json:"restoreGeneration,omitempty"`
	BaseAttempt       int64               `json:"baseAttempt"`
	SchedulePaused    bool                `json:"schedulePaused"`
}

// ArchiveGeneration is immutable retained history; the writer receives only the current prefix.
type ArchiveGeneration struct {
	Generation int64  `json:"generation"`
	Prefix     string `json:"prefix"`
}
