package v1alpha1

// ReleaseSelection reuses admitted inputs for one exact existing nonproduction revision.
type ReleaseSelection struct {
	ReleaseID     string `json:"releaseId"`
	EnvironmentID string `json:"environmentId"`
	Generation    int64  `json:"generation"`
	RequestKey    string `json:"requestKey"`
}

// OperationMutation binds retry or cancellation to the observed desired generation.
type OperationMutation struct {
	OperationID string `json:"operationId"`
	Generation  int64  `json:"generation"`
	RequestKey  string `json:"requestKey"`
}
