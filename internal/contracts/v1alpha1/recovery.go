package v1alpha1

// RecoveryCheckpoint is durable cluster evidence that outlives an environment.
type RecoveryCheckpoint struct {
	Scope
	EnvironmentID string `json:"environmentId"`
	Generation    int64  `json:"generation"`
	Action        string `json:"action"`
	DeploymentID  string `json:"deploymentId,omitempty"`
	Bundle        string `json:"bundle,omitempty"`
}

// RetainedDatabaseCheckpoint records independently surviving data identity.
type RetainedDatabaseCheckpoint struct {
	Scope
	ID         string `json:"id"`
	Generation int64  `json:"generation"`
	UID        string `json:"uid"`
}
