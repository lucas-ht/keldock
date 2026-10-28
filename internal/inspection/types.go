// Package inspection provides the shared, scoped read model for the console and MCP.
package inspection

import (
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Page is a bounded keyset response; a cursor grants no additional access.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor"`
}

// Project summarizes current desired environments, never credentials or application settings.
type Project struct {
	v1.Project
	Environments int `json:"environments"`
	Ready        int `json:"ready"`
	Attention    int `json:"attention"`
}

// Environment reports one exact desired revision and the age of evidence for that revision.
type Environment struct {
	URL                   string            `json:"url,omitempty"`
	PullRequestHead       string            `json:"pullRequestHead,omitempty"`
	PullRequestClosed     bool              `json:"pullRequestClosed"`
	ID                    string            `json:"id"`
	ProjectID             string            `json:"projectId"`
	Application           string            `json:"application"`
	ClusterID             string            `json:"clusterId"`
	Kind                  string            `json:"kind"`
	DeletionReason        string            `json:"deletionReason,omitempty"`
	PullRequest           int64             `json:"pullRequest"`
	Generation            int64             `json:"generation"`
	Deleted               bool              `json:"deleted"`
	ExpiresAt             *time.Time        `json:"expiresAt,omitempty"`
	Operation             v1.Operation      `json:"operation"`
	ReleaseID             string            `json:"releaseId"`
	Hostname              string            `json:"hostname"`
	Source                *v1.Source        `json:"source,omitempty"`
	Images                map[string]string `json:"images"`
	Components            []Component       `json:"components"`
	Observation           *v1.Observation   `json:"observation"`
	ObservationAgeSeconds float64           `json:"observationAgeSeconds"`
	Stale                 bool              `json:"stale"`
}

// Component exposes declared topology, excluding environment values, commands and credentials.
type Component struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Image     string   `json:"image"`
	DependsOn []string `json:"dependsOn"`
}

// Status retains the original CI receipt response shape with redacted bounded errors.
type Status struct {
	APIVersion            string          `json:"apiVersion"`
	Operation             v1.Operation    `json:"operation"`
	Observation           *v1.Observation `json:"observation"`
	ObservationAgeSeconds float64         `json:"observationAgeSeconds"`
	Stale                 bool            `json:"stale"`
}

// Event is durable selection/operation history; it does not claim a Kubernetes event stream.
type Event struct {
	ID         int64     `json:"id"`
	Actor      string    `json:"actor"`
	Event      string    `json:"event"`
	Generation int64     `json:"generation"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Diagnostics is revision-bound evidence, not arbitrary pod logs or shell output.
type Diagnostics struct {
	EnvironmentID         string               `json:"environmentId"`
	Generation            int64                `json:"generation"`
	State                 string               `json:"state"`
	Stale                 bool                 `json:"stale"`
	ObservationAgeSeconds float64              `json:"observationAgeSeconds"`
	PublicationError      string               `json:"publicationError,omitempty"`
	Components            []v1.ComponentHealth `json:"components"`
}
