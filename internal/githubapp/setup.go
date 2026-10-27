package githubapp

import (
	"context"
	"net/url"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
)

// Setup contains only a GitHub destination and public registration parameters.
type Setup struct {
	URL      string         `json:"url"`
	Manifest map[string]any `json:"manifest,omitempty"`
}

// BeginSetup starts installation or manifest registration without granting repository access.
func (service *Service) BeginSetup(ctx context.Context, principal access.Principal, expected int64, purpose string) (Setup, error) {
	app, err := service.Repository.State(ctx)
	if err != nil {
		return Setup{}, err
	}
	if purpose == "manifest" && app.Enabled {
		return Setup{}, access.ErrConflict
	}
	if purpose == "installation" && (!app.Enabled || app.Slug == "") {
		return Setup{}, access.ErrInvalid
	}
	state, err := service.Repository.BeginSetup(ctx, principal, expected, purpose)
	if err != nil {
		return Setup{}, err
	}
	if purpose == "installation" {
		return Setup{URL: "https://github.com/apps/" + url.PathEscape(app.Slug) + "/installations/new?state=" + url.QueryEscape(state)}, nil
	}
	origin := service.Config.PublicURL
	return Setup{URL: "https://github.com/settings/apps/new?state=" + url.QueryEscape(state), Manifest: map[string]any{
		"name": "Keldock " + service.Repository.Organization, "url": origin, "description": "Application delivery on customer-controlled infrastructure",
		"public": false, "request_oauth_on_install": false, "setup_on_update": true, "redirect_url": origin + "/settings/github/callback",
		"setup_url": origin + "/settings/github/callback", "hook_attributes": map[string]any{"url": origin + "/v1/github/webhooks", "active": true},
		"default_permissions": RequiredPermissions(), "default_events": []string{"pull_request", "push", "repository", "workflow_run"}}}, nil
}

// CompleteSetup rechecks the initiating administrator before exchanging a single-use code.
// Installation IDs from query strings never become grants; Sync reads GitHub's actual selection.
func (service *Service) CompleteSetup(ctx context.Context, principal access.Principal, state, purpose, code string) error {
	version, err := service.Repository.ConsumeSetup(ctx, principal, state, purpose)
	if err != nil {
		return err
	}
	if purpose == "manifest" {
		app, err := client.ConvertManifest(ctx, code, service.Config.Client)
		if err != nil {
			return err
		}
		return service.Configure(ctx, principal, version, app)
	}
	return service.Sync(ctx)
}

// ConfigureWebhook keeps the webhook secret inside the installation and GitHub App boundary.
func (service *Service) ConfigureWebhook(ctx context.Context, principal access.Principal, expected int64) error {
	return service.Repository.AdminAction(ctx, principal, expected, "github.webhook_configured", func() error {
		api, app, err := service.active(ctx)
		if err != nil {
			return err
		}
		return api.ConfigureWebhook(ctx, service.Config.PublicURL+"/v1/github/webhooks", app.WebhookSecret)
	})
}
