package v1alpha1

import (
	"encoding/json"
	"time"
)

// Placement is trusted environment configuration resolved by the control plane.
type Placement struct {
	EnvironmentID string                     `json:"environmentId"`
	DeploymentID  string                     `json:"deploymentId"`
	Generation    int64                      `json:"generation"`
	Namespace     string                     `json:"namespace"`
	Hostname      string                     `json:"hostname,omitempty"`
	Cloud         map[string]WorkloadCloud   `json:"cloud,omitempty"`
	Domains       []DomainRoute              `json:"domains,omitempty"`
	Databases     map[string]DatabaseBinding `json:"databases,omitempty"`
}

// ProjectedDeployment is the approved, version-bound input for a scoped cluster agent.
type ProjectedDeployment struct {
	Operation  Operation         `json:"operation"`
	Selection  Placement         `json:"selection"`
	Definition Application       `json:"definition"`
	Images     map[string]string `json:"images"`
	Bundle     string            `json:"bundle"`
}

// Source preserves GitHub's immutable repository, workflow and tested source identity.
type Source struct {
	RepositoryID int64  `json:"repositoryId"`
	OwnerID      int64  `json:"ownerId"`
	Repository   string `json:"repository"`
	Commit       string `json:"commit"`
	HeadCommit   string `json:"headCommit,omitempty"`
	Ref          string `json:"ref"`
	Event        string `json:"event"`
	WorkflowRef  string `json:"workflowRef"`
	RunID        int64  `json:"runId"`
	RunAttempt   int64  `json:"runAttempt"`
	PullRequest  int64  `json:"pullRequest,omitempty"`
}

// SubmitRelease admits a complete prebuilt application; it never executes a build.
type SubmitRelease struct {
	APIVersion string            `json:"apiVersion"`
	ProjectID  string            `json:"projectId"`
	Definition Application       `json:"definition"`
	Source     Source            `json:"source"`
	Images     map[string]string `json:"images"`
	Provenance json.RawMessage   `json:"provenance"`
}

// Release is an immutable admitted input snapshot and its original evidence.
type Release struct {
	Scope
	ID         string        `json:"id"`
	InputHash  string        `json:"inputHash"`
	Input      SubmitRelease `json:"input"`
	AdmittedAt time.Time     `json:"admittedAt"`
}

// Environment keeps a stable target and tombstone independently of individual releases.
type Environment struct {
	Scope
	ID           string    `json:"id"`
	Application  string    `json:"application"`
	ClusterID    string    `json:"clusterId"`
	RepositoryID int64     `json:"repositoryId"`
	PullRequest  int64     `json:"pullRequest,omitempty"`
	Generation   int64     `json:"generation"`
	Deleted      bool      `json:"deleted"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
}

// Deployment fixes the selected inputs and desired generation for an environment.
type Deployment struct {
	Scope
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	ReleaseID     string `json:"releaseId"`
	Generation    int64  `json:"generation"`
	ConfigHash    string `json:"configHash"`
	Bundle        string `json:"bundle,omitempty"`
}

// Receipt separates durable acceptance from completion and survives client interruption.
type Receipt struct {
	APIVersion    string `json:"apiVersion"`
	ReleaseID     string `json:"releaseId"`
	EnvironmentID string `json:"environmentId"`
	DeploymentID  string `json:"deploymentId"`
	OperationID   string `json:"operationId"`
	Generation    int64  `json:"generation"`
	StatusURL     string `json:"statusUrl"`
}

// Operation records durable execution state; an attempt is not a new desired selection.
type Operation struct {
	Scope
	ID            string    `json:"id"`
	DeploymentID  string    `json:"deploymentId"`
	EnvironmentID string    `json:"environmentId"`
	Generation    int64     `json:"generation"`
	Kind          string    `json:"kind"`
	State         string    `json:"state"`
	Attempt       int64     `json:"attempt"`
	Error         string    `json:"error,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Observation reports evidence for one exact selection, including its freshness.
type Observation struct {
	EnvironmentID string            `json:"environmentId"`
	DeploymentID  string            `json:"deploymentId"`
	Generation    int64             `json:"generation"`
	Bundle        string            `json:"bundle"`
	State         string            `json:"state"`
	Reason        string            `json:"reason,omitempty"`
	Components    []ComponentHealth `json:"components"`
	ObservedAt    time.Time         `json:"observedAt"`
}

// ComponentHealth ties controller readiness to reconciled object versions and source.
type ComponentHealth struct {
	Name               string `json:"name"`
	Ready              bool   `json:"ready"`
	Reason             string `json:"reason"`
	Message            string `json:"message,omitempty"`
	ObjectUID          string `json:"objectUid,omitempty"`
	ResourceVersion    string `json:"resourceVersion,omitempty"`
	WorkloadUID        string `json:"workloadUid,omitempty"`
	WorkloadVersion    string `json:"workloadVersion,omitempty"`
	Image              string `json:"image,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration"`
	AppliedRevision    string `json:"appliedRevision,omitempty"`
}

// Enrollment exchanges a single-use installation grant for a revocable agent identity.
type Enrollment struct {
	ClusterID string `json:"clusterId"`
	Token     string `json:"token"`
}

// AgentIdentity is a server-issued credential restricted to one organization and cluster.
type AgentIdentity struct {
	OrganizationID string    `json:"organizationId"`
	ClusterID      string    `json:"clusterId"`
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expiresAt"`
}
