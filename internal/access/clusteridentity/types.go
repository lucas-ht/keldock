package clusteridentity

import "time"

// Cluster reports connection metadata, never Kubernetes or agent credentials.
type Cluster struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Revision   int64      `json:"revision"`
	State      string     `json:"state"`
	AgentID    string     `json:"agentId,omitempty"`
	Version    string     `json:"agentVersion,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

// Enrollment is a short-lived one-time enrollment grant for exactly one cluster.
type Enrollment struct {
	ClusterID string    `json:"clusterId"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Identity carries an agent credential only in exchange/rotation responses.
type Identity struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	ClusterID      string    `json:"clusterId"`
	Generation     int64     `json:"generation"`
	Token          string    `json:"token,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt"`
}
