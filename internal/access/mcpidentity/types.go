package mcpidentity

import "time"

// Settings is the administrator-managed endpoint switch and its optimistic revision.
type Settings struct {
	Enabled  bool   `json:"enabled"`
	Version  int64  `json:"version"`
	Endpoint string `json:"endpoint"`
}

// Client is public registration metadata, never a confidential OAuth credential.
type Client struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	RedirectURIs []string   `json:"redirectUris"`
	CreatedAt    time.Time  `json:"createdAt"`
	DisabledAt   *time.Time `json:"disabledAt,omitempty"`
}

// AuthorizationRequest describes what a signed-in person is being asked to approve.
type AuthorizationRequest struct {
	ID          string    `json:"id"`
	ClientID    string    `json:"clientId"`
	ClientName  string    `json:"clientName"`
	RedirectURI string    `json:"redirectUri"`
	Resource    string    `json:"resource"`
	Scope       string    `json:"scope"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// AuthorizationInput is validated before any browser redirect is trusted.
type AuthorizationInput struct {
	ClientID        string
	RedirectURI     string
	State           string
	Challenge       string
	ChallengeMethod string
	Resource        string
	Scope           string
	ResponseType    string
}

// TokenPair is returned only to a PKCE-proving client at the token endpoint.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// Connection is safe grant metadata shown to its owner or an administrator.
type Connection struct {
	ID         string     `json:"id"`
	ClientID   string     `json:"clientId"`
	ClientName string     `json:"clientName"`
	UserID     string     `json:"userId"`
	UserName   string     `json:"userName"`
	Projects   []string   `json:"projects"`
	Scope      string     `json:"scope"`
	Resource   string     `json:"resource"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}
