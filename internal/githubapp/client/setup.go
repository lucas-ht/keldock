package client

import (
	"context"
	"fmt"
	"regexp"

	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
)

var manifestCode = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,128}$`)

// ConvertManifest exchanges GitHub's single-use code without exposing generated secrets.
func ConvertManifest(ctx context.Context, code string, options Options) (credentials.App, error) {
	if !manifestCode.MatchString(code) {
		return credentials.App{}, fmt.Errorf("invalid GitHub manifest code")
	}
	api, err := newConnection(options)
	if err != nil {
		return credentials.App{}, err
	}
	var result struct {
		ID            int64  `json:"id"`
		ClientID      string `json:"client_id"`
		Slug          string `json:"slug"`
		PEM           string `json:"pem"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := api.request(ctx, "POST", "/app-manifests/"+code+"/conversions", "", nil, &result); err != nil {
		return credentials.App{}, err
	}
	app := credentials.App{ID: result.ID, ClientID: result.ClientID, Slug: result.Slug, PrivateKey: result.PEM, WebhookSecret: result.WebhookSecret}
	if _, err := app.Key(); err != nil {
		return credentials.App{}, fmt.Errorf("invalid generated GitHub App credentials")
	}
	return app, nil
}

// ConfigureWebhook updates the App-owned signed endpoint through JWT authentication.
func (client *Client) ConfigureWebhook(ctx context.Context, endpoint, secret string) error {
	return client.AppRequest(ctx, "PATCH", "/app/hook/config", map[string]string{"url": endpoint, "content_type": "json", "insecure_ssl": "0", "secret": secret}, nil)
}
