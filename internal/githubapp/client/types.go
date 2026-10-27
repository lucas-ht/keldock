// Package client implements the bounded GitHub.com App REST profile.
package client

import "time"

// APIVersion pins the tested REST contract independently of GitHub's default version.
const APIVersion = "2026-03-10"

// Account identifies a stable GitHub user or organization.
type Account struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

// App exposes registration metadata, never private credentials.
type App struct {
	ID          int64             `json:"id"`
	ClientID    string            `json:"client_id"`
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Owner       Account           `json:"owner"`
	Permissions map[string]string `json:"permissions"`
	Events      []string          `json:"events"`
}

// Installation is GitHub's current account-level delegation to this App.
type Installation struct {
	ID          int64             `json:"id"`
	AppID       int64             `json:"app_id"`
	Account     Account           `json:"account"`
	SuspendedAt *time.Time        `json:"suspended_at"`
	Permissions map[string]string `json:"permissions"`
}

// Repository uses stable IDs for authority and the current name for API routing.
type Repository struct {
	ID            int64   `json:"id"`
	FullName      string  `json:"full_name"`
	Owner         Account `json:"owner"`
	DefaultBranch string  `json:"default_branch"`
	Private       bool    `json:"private"`
}

// PullRequest preserves the tested merge commit separately from the source head.
type PullRequest struct {
	Number    int64     `json:"number"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
	Head      struct {
		SHA  string     `json:"sha"`
		Repo Repository `json:"repo"`
	} `json:"head"`
}

// Delivery identifies a webhook result that can be inspected and redelivered.
type Delivery struct {
	ID             int64     `json:"id"`
	GUID           string    `json:"guid"`
	DeliveredAt    time.Time `json:"delivered_at"`
	StatusCode     int       `json:"status_code"`
	Event          string    `json:"event"`
	Redelivery     bool      `json:"redelivery"`
	InstallationID int64     `json:"installation_id"`
	RepositoryID   int64     `json:"repository_id"`
}
