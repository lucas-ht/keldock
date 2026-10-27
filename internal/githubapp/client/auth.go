package client

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// JWT creates a short-lived App identity; it is never a repository or human identity.
func (client *Client) JWT(now time.Time) (string, error) {
	key, err := client.credentials.Key()
	if err != nil {
		return "", err
	}
	issuer := client.credentials.ClientID
	if issuer == "" {
		issuer = strconv.FormatInt(client.credentials.ID, 10)
	}
	claims, err := json.Marshal(map[string]any{"iat": now.Add(-30 * time.Second).Unix(), "exp": now.Add(8 * time.Minute).Unix(), "iss": issuer})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App identity")
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// AppRequest authenticates an App-management request with a fresh short-lived JWT.
func (client *Client) AppRequest(ctx context.Context, method, path string, input, output any) error {
	token, err := client.JWT(time.Now())
	if err != nil {
		return err
	}
	return client.request(ctx, method, path, token, input, output)
}

// Token is a short-lived installation credential. Callers must not serialize it publicly.
type Token struct {
	Value     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InstallationToken narrows an installation to explicit repository IDs when supplied.
func (client *Client) InstallationToken(ctx context.Context, installation int64, repositories []int64) (Token, error) {
	var token Token
	if installation <= 0 || len(repositories) > 32 {
		return token, fmt.Errorf("invalid installation token scope")
	}
	for _, id := range repositories {
		if id <= 0 {
			return token, fmt.Errorf("invalid repository scope")
		}
	}
	input := map[string]any{}
	if len(repositories) > 0 {
		input["repository_ids"] = repositories
	}
	err := client.AppRequest(ctx, "POST", fmt.Sprintf("/app/installations/%d/access_tokens", installation), input, &token)
	if err == nil && (token.Value == "" || token.ExpiresAt.Before(time.Now().Add(time.Minute))) {
		err = fmt.Errorf("GitHub returned an invalid installation credential")
	}
	return token, err
}

// RepositoryRequest uses the caller's current installation token; cached tokens never grant authority.
func (client *Client) RepositoryRequest(ctx context.Context, token Token, method, path string, input, output any) error {
	if token.Value == "" || token.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("GitHub installation credential expired")
	}
	return client.request(ctx, method, path, token.Value, input, output)
}
