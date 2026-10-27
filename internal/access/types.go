// Package access owns human, cluster and MCP identity without depending on an identity provider.
package access

import (
	"errors"
	"slices"
	"time"
)

// Expected identity errors never include credentials or password material.
var (
	ErrDenied      = errors.New("access denied")
	ErrNotFound    = errors.New("identity not found")
	ErrConflict    = errors.New("conflicting identity revision")
	ErrInvalid     = errors.New("invalid identity request")
	ErrLimited     = errors.New("authentication rate limit")
	ErrSessionBusy = errors.New("cluster agent session is already active")
)

// User contains safe account metadata; password material stays in private repository rows.
type User struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	Projects       []string   `json:"projects"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastLoginAt    *time.Time `json:"lastLoginAt,omitempty"`
}

// Principal is authenticated authority, narrowed further for MCP connections.
type Principal struct {
	User          User
	Kind          string
	GrantID       string
	TokenProjects []string
}

// CanRead checks current account grants and the credential's narrower project scope.
func (principal Principal) CanRead(project string) bool {
	if principal.User.Status != "active" || (principal.Kind == "mcp" && !slices.Contains(principal.TokenProjects, project)) {
		return false
	}
	return principal.User.Role == "admin" || slices.Contains(principal.User.Projects, project)
}

// CanOperate permits human operators to act only in their current project scope.
func (principal Principal) CanOperate(project string) bool {
	return principal.Kind == "session" && principal.CanRead(project) && (principal.User.Role == "admin" || principal.User.Role == "operator")
}

// IsAdmin deliberately excludes machine/read credentials, even when their owner is an admin.
func (principal Principal) IsAdmin() bool {
	return principal.Kind == "session" && principal.User.Status == "active" && principal.User.Role == "admin"
}

// Invitation contains a one-time secret returned only by an explicit operator action.
type Invitation struct {
	User      User      `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// UserUpdate carries optimistic concurrency for administrative account changes.
type UserUpdate struct {
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Status   string   `json:"status"`
	Projects []string `json:"projects"`
	Version  int64    `json:"version"`
}
