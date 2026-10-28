// Package cloud defines the bounded optional cloud lifecycle, independently of workload delivery.
package cloud

import (
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Resource kinds have fixed provider implementations; they are not executable plugins.
const (
	ObjectStorage = "object-storage"
	BackupStore   = "backup-store"
	Domain        = "domain"
)

// Connection extends the project-scoped cloud contract with revision-bound authority and evidence.
type Connection struct {
	v1.CloudConnection
	ClusterID   string                 `json:"clusterId"`
	Generation  int64                  `json:"generation"`
	Enabled     bool                   `json:"enabled"`
	Observation *ConnectionObservation `json:"observation,omitempty"`
}

// ConnectionObservation distinguishes verified identity from unproven resource permissions.
type ConnectionObservation struct {
	Generation      int64     `json:"generation"`
	State           string    `json:"state"`
	Reason          string    `json:"reason"`
	VerifiedAccount string    `json:"verifiedAccount,omitempty"`
	ObservedAt      time.Time `json:"observedAt"`
}

// ConnectionRequest cannot choose tenant ownership, raw IAM policies, or provider credentials.
type ConnectionRequest struct {
	Provider    string   `json:"provider"`
	Account     string   `json:"account"`
	Region      string   `json:"region"`
	Profiles    []string `json:"profiles"`
	IdentityRef string   `json:"identityRef"`
	Generation  int64    `json:"generation"`
}

// Resource has ownership and retention independent of disposable application namespaces.
type Resource struct {
	v1.Resource
	ConnectionID         string               `json:"connectionId"`
	ConnectionGeneration int64                `json:"connectionGeneration"`
	Kind                 string               `json:"kind"`
	Mode                 string               `json:"mode"`
	ExternalName         string               `json:"externalName"`
	Owner                string               `json:"owner"`
	AdoptionGeneration   int64                `json:"adoptionGeneration,omitempty"`
	AdoptionReviewHash   string               `json:"adoptionReviewHash,omitempty"`
	State                string               `json:"state"`
	Observation          *ResourceObservation `json:"observation,omitempty"`
}

// ResourceObservation is usable evidence for an exact resource and connection revision.
type ResourceObservation struct {
	Generation           int64     `json:"generation"`
	ConnectionGeneration int64     `json:"connectionGeneration"`
	State                string    `json:"state"`
	Reason               string    `json:"reason"`
	ExternalID           string    `json:"externalId,omitempty"`
	ObservedAt           time.Time `json:"observedAt"`
}

// ResourceRequest creates new storage or observes existing storage; adoption is a separate review.
type ResourceRequest struct {
	ConnectionID string `json:"connectionId"`
	Kind         string `json:"kind"`
	Profile      string `json:"profile"`
	Mode         string `json:"mode"`
	ExistingName string `json:"existingName,omitempty"`
}

// BindingRequest names exact approved consumers, without choosing AWS roles or Kubernetes identities.
type BindingRequest struct {
	ResourceID    string   `json:"resourceId"`
	EnvironmentID string   `json:"environmentId"`
	Components    []string `json:"components"`
	Access        string   `json:"access"`
	Generation    int64    `json:"generation"`
}

// ConsumerIdentity is resolved by trusted policy rather than application source.
type ConsumerIdentity struct {
	Component      string `json:"component"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
	RoleARN        string `json:"roleArn"`
	Prefix         string `json:"prefix"`
}

// Binding records grants separately from the external resource's management ownership.
type Binding struct {
	v1.ResourceBinding
	ConnectionGeneration int64              `json:"connectionGeneration"`
	Access               string             `json:"access"`
	Consumers            []ConsumerIdentity `json:"consumers"`
	State                string             `json:"state"`
}

// DomainRequest claims one routed component; DNS targets and issuers come from operator policy.
type DomainRequest struct {
	ConnectionID  string `json:"connectionId"`
	Profile       string `json:"profile"`
	EnvironmentID string `json:"environmentId"`
	Component     string `json:"component"`
	Hostname      string `json:"hostname"`
	Generation    int64  `json:"generation"`
}

// DomainClaim keeps DNS ownership, DNS convergence and certificate health distinct.
type DomainClaim struct {
	v1.Scope
	ID                   string        `json:"id"`
	Generation           int64         `json:"generation"`
	ConnectionGeneration int64         `json:"connectionGeneration"`
	Request              DomainRequest `json:"request"`
	OwnershipVerified    bool          `json:"ownershipVerified"`
	DNSReady             bool          `json:"dnsReady"`
	CertificateReady     bool          `json:"certificateReady"`
	State                string        `json:"state"`
	ObservedAt           *time.Time    `json:"observedAt,omitempty"`
}
