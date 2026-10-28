// Package v1alpha1 defines the first versioned Keldock wire contracts.
// This API is experimental; compatibility changes require an explicit version change.
package v1alpha1

// APIVersion identifies the experimental wire format.
const APIVersion = "keldock.io/v1alpha1"

// Application describes runtime inputs, independently of builds and Kubernetes authority.
type Application struct {
	APIVersion string                     `json:"apiVersion" yaml:"apiVersion"`
	Name       string                     `json:"name" yaml:"name"`
	Components map[string]Component       `json:"components" yaml:"components"`
	Resources  map[string]ResourceRequest `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// Component is a bounded workload and its explicit runtime dependencies.
type Component struct {
	Kind       string            `json:"kind" yaml:"kind"`
	Command    []string          `json:"command,omitempty" yaml:"command,omitempty"`
	Args       []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Port       int32             `json:"port,omitempty" yaml:"port,omitempty"`
	Probe      *Probe            `json:"probe,omitempty" yaml:"probe,omitempty"`
	Schedule   string            `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	DependsOn  []string          `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	ConnectsTo []string          `json:"connectsTo,omitempty" yaml:"connectsTo,omitempty"`
	Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Size       string            `json:"size" yaml:"size"`
	Route      bool              `json:"route,omitempty" yaml:"route,omitempty"`
	Resources  []string          `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// Probe is an HTTP or exec readiness/liveness check. Exactly one form is allowed.
type Probe struct {
	Path    string   `json:"path,omitempty" yaml:"path,omitempty"`
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
}

// Scope is server-resolved ownership. Request routing never establishes authority alone.
type Scope struct {
	OrganizationID string `json:"organizationId"`
	ProjectID      string `json:"projectId"`
}

// Organization is an installation's explicit authenticated ownership root.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Project binds a repository and cluster placement to its owning organization.
type Project struct {
	Scope
	Name         string        `json:"name"`
	RepositoryID int64         `json:"repositoryId"`
	ClusterID    string        `json:"clusterId"`
	Previews     PreviewPolicy `json:"previews"`
}
