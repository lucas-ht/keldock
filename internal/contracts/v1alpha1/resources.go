package v1alpha1

// CapabilityDefinition describes an approved integration, not executable plugin code.
type CapabilityDefinition struct {
	APIVersion    string   `json:"apiVersion"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Digest        string   `json:"digest"`
	Dependencies  []string `json:"dependencies"`
	Permissions   []string `json:"permissions"`
	Licenses      []string `json:"licenses"`
	Configuration string   `json:"configurationSchema"`
	Binding       string   `json:"bindingSchema"`
}

// CapabilityInstallation separates supplied, managed and external ownership.
// Declaring this contract does not expose an installation API in the first runtime.
type CapabilityInstallation struct {
	OrganizationID string `json:"organizationId"`
	ID             string `json:"id"`
	Definition     string `json:"definition"`
	Version        string `json:"version"`
	ClusterID      string `json:"clusterId"`
	Mode           string `json:"mode"`
	Owner          string `json:"owner"`
	Generation     int64  `json:"generation"`
}

// Resource retains project data independently of disposable deployment namespaces.
type Resource struct {
	Scope
	ID             string `json:"id"`
	InstallationID string `json:"installationId"`
	Profile        string `json:"profile"`
	Generation     int64  `json:"generation"`
	Retention      string `json:"retention"`
}

// ResourceBinding is an explicit consumer grant; it contains no plaintext credentials.
type ResourceBinding struct {
	Scope
	ID            string   `json:"id"`
	ResourceID    string   `json:"resourceId"`
	EnvironmentID string   `json:"environmentId"`
	Components    []string `json:"components"`
	SecretRef     string   `json:"secretRef,omitempty"`
	IdentityRef   string   `json:"identityRef,omitempty"`
	Generation    int64    `json:"generation"`
}

// CloudConnection restricts an optional provisioning identity to approved profiles.
type CloudConnection struct {
	Scope
	ID          string   `json:"id"`
	Provider    string   `json:"provider"`
	Account     string   `json:"account"`
	Region      string   `json:"region"`
	Profiles    []string `json:"profiles"`
	IdentityRef string   `json:"identityRef"`
}
