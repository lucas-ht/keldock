package v1alpha1

// WorkloadCloud is trusted placement, never accepted through application source or CI.
type WorkloadCloud struct {
	ConnectionID         string                        `json:"connectionId"`
	ConnectionGeneration int64                         `json:"connectionGeneration"`
	ServiceAccount       string                        `json:"serviceAccount"`
	RoleARN              string                        `json:"roleArn"`
	Region               string                        `json:"region"`
	EgressCIDRs          []string                      `json:"egressCidrs"`
	Stores               map[string]ObjectStoreBinding `json:"stores"`
}

// ObjectStoreBinding exposes a bucket and bounded consumer prefix without static credentials.
type ObjectStoreBinding struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
	Access string `json:"access"`
}

// DomainRoute is a separately approved hostname and operator-chosen certificate issuer.
type DomainRoute struct {
	ConnectionID         string `json:"connectionId"`
	ConnectionGeneration int64  `json:"connectionGeneration"`
	ID                   string `json:"id"`
	Component            string `json:"component"`
	Hostname             string `json:"hostname"`
	IssuerRef            string `json:"issuerRef"`
}
