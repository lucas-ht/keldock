// Package githubapp coordinates installation-owned GitHub delegation and durable work.
package githubapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
	"github.com/lucas-ht/keldock/internal/githubapp/store"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Config is operator-owned installation policy, not application-supplied authority.
type Config struct {
	CredentialsFile  string
	PublicURL        string
	Development      bool
	Projects         map[string]v1.Project
	RepositoryOwners map[int64]int64
	Client           client.Options
}

// Service shares the product's durable state and identity boundary.
type Service struct {
	Repository *store.Repository
	Product    *postgres.Store
	File       credentials.File
	Config     Config
}

// New validates installation placement and constructs no remote resources.
func New(product *postgres.Store, users *access.Repository, config Config) (*Service, error) {
	parsed, err := url.Parse(config.PublicURL)
	if product == nil || users == nil || err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || config.CredentialsFile == "" {
		return nil, access.ErrInvalid
	}
	localOrigin := config.Development && parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost")
	if parsed.Scheme != "https" && !localOrigin {
		return nil, access.ErrInvalid
	}
	config.PublicURL = strings.TrimSuffix(config.PublicURL, "/")
	for id, project := range config.Projects {
		if id != project.ProjectID || project.OrganizationID != users.Organization || config.RepositoryOwners[project.RepositoryID] <= 0 {
			return nil, access.ErrInvalid
		}
	}
	repository, err := store.New(product.Pool(), users)
	if err != nil {
		return nil, err
	}
	return &Service{Repository: repository, Product: product, File: credentials.File{Path: config.CredentialsFile}, Config: config}, nil
}

func fingerprint(app credentials.App) string {
	sum := sha256.Sum256([]byte(app.PrivateKey + "\x00" + app.WebhookSecret))
	return hex.EncodeToString(sum[:])
}

// Initialize establishes configured identity without depending on GitHub availability.
func (service *Service) Initialize(ctx context.Context) error {
	app, err := service.File.Read()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return service.Repository.Seed(ctx, client.App{ID: app.ID, ClientID: app.ClientID, Slug: app.Slug}, fingerprint(app))
}

func (service *Service) active(ctx context.Context) (*client.Client, credentials.App, error) {
	app, err := service.File.Read()
	if err != nil {
		return nil, app, access.ErrDenied
	}
	state, err := service.Repository.State(ctx)
	if err != nil {
		return nil, app, err
	}
	if !state.Enabled || state.AppID != app.ID {
		return nil, app, access.ErrDenied
	}
	api, err := client.New(app, service.Config.Client)
	return api, app, err
}

// Status returns only safe configuration, delegation and queue health.
type Status struct {
	App                 store.AppState          `json:"app"`
	Installations       []store.Installation    `json:"installations"`
	Repositories        []store.RepositoryGrant `json:"repositories"`
	PendingEvents       int                     `json:"pendingEvents"`
	FailedEvents        int                     `json:"failedEvents"`
	RequiredPermissions map[string]string       `json:"requiredPermissions"`
	WebhookURL          string                  `json:"webhookUrl"`
	SetupURL            string                  `json:"setupUrl"`
}

// RequiredPermissions is the first App's minimum repository permission profile.
func RequiredPermissions() map[string]string {
	return map[string]string{"contents": "read", "actions": "read", "pull_requests": "write", "checks": "write", "deployments": "write"}
}

// Status exposes the same installation state that gates release admission.
func (service *Service) Status(ctx context.Context) (Status, error) {
	status := Status{RequiredPermissions: RequiredPermissions(), WebhookURL: service.Config.PublicURL + "/v1/github/webhooks", SetupURL: service.Config.PublicURL + "/settings/github/callback"}
	var err error
	status.App, err = service.Repository.State(ctx)
	if err != nil {
		return status, err
	}
	status.Installations, status.Repositories, err = service.Repository.Inventory(ctx)
	if err != nil {
		return status, err
	}
	status.PendingEvents, status.FailedEvents, err = service.Repository.EventCounts(ctx)
	return status, err
}

// Configure verifies a supplied existing App before atomically persisting private material.
func (service *Service) Configure(ctx context.Context, principal access.Principal, expected int64, app credentials.App) error {
	api, err := client.New(app, service.Config.Client)
	if err != nil {
		return access.ErrInvalid
	}
	metadata, err := api.App(ctx)
	if err != nil {
		return err
	}
	return service.Repository.Configure(ctx, principal, expected, metadata, fingerprint(app), func() error { return service.File.Write(app) })
}

// Disconnect revokes this installation's delegation while retaining GitHub and workload state.
func (service *Service) Disconnect(ctx context.Context, principal access.Principal, expected int64) error {
	return service.Repository.Disable(ctx, principal, expected, func() error {
		err := service.File.Remove()
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}
