package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	"github.com/lucas-ht/keldock/internal/admission"
	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/cloud"
	cloudstore "github.com/lucas-ht/keldock/internal/cloud/store"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/lucas-ht/keldock/internal/controlplane/authentication"
	"github.com/lucas-ht/keldock/internal/githubapp"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/validation"
)

func serve(ctx context.Context, settings controlSettings, migrate bool) error {
	if !v1.ValidName(settings.Organization) || !v1.ValidName(settings.Cluster) || len(validation.IsDNS1123Subdomain(settings.Domain)) != 0 || settings.Listen == "" || len(settings.Projects) > 32 {
		return fmt.Errorf("invalid installation, domain, listen address or project configuration")
	}
	if settings.RegistryPlainHTTP && !settings.Development {
		return fmt.Errorf("plain HTTP registry is a development-only profile")
	}
	webhookSecret, err := secret(settings.WebhookSecretFile)
	if err != nil {
		return err
	}
	store, err := postgres.Open(ctx, settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if migrate {
		if err := store.Migrate(ctx); err != nil {
			return err
		}
	}
	if fenced, err := store.RecoveryFenced(ctx, settings.Organization); err != nil {
		return err
	} else if fenced {
		return fmt.Errorf("restored installation is fenced; run recovery review and resume against the surviving cluster before serving")
	}
	projects := map[string]v1.Project{}
	owners := map[int64]int64{}
	var policies []admission.RepositoryPolicy
	for _, item := range settings.Projects {
		if _, exists := projects[item.ID]; exists {
			return fmt.Errorf("duplicate configured project")
		}
		project := v1.Project{Scope: v1.Scope{OrganizationID: settings.Organization, ProjectID: item.ID}, Name: item.Name, RepositoryID: item.RepositoryID, ClusterID: settings.Cluster, Previews: item.Previews}
		if err := store.SeedProject(ctx, project); err != nil {
			return err
		}
		policy, err := settings.policy(item)
		if err != nil {
			return err
		}
		policies = append(policies, policy)
		projects[item.ID] = project
		owners[item.RepositoryID] = item.OwnerID
	}
	var verifier *admission.GitHub
	if settings.GitHubIssuer != "" || settings.GitHubAudience != "" {
		verifier, err = admission.NewGitHub(ctx, settings.GitHubIssuer, settings.GitHubAudience, policies, settings.Development)
		if err != nil {
			return err
		}
	}
	var password string
	if settings.RegistryPasswordFile != "" {
		password, err = credential(settings.RegistryPasswordFile, 1)
		if err != nil {
			return err
		}
	}
	publisher, err := artifacts.NewPublisher(artifacts.RegistryConfig{PushRepository: settings.RegistryPush, PullRepository: settings.RegistryPull, PlainHTTP: settings.RegistryPlainHTTP, Username: settings.RegistryUsername, Password: password})
	if err != nil {
		return err
	}
	users, err := access.NewRepository(store.Pool(), settings.Organization)
	if err != nil {
		return err
	}
	clusters, err := clusteridentity.NewRepository(store.Pool(), users, settings.Cluster)
	if err != nil {
		return err
	}
	if err := clusters.Seed(ctx, settings.Cluster); err != nil {
		return err
	}
	mcp, err := mcpidentity.NewRepository(store.Pool(), users, settings.PublicURL)
	if err != nil {
		return err
	}
	if err := mcp.Seed(ctx); err != nil {
		return err
	}
	projectIDs := make([]string, 0, len(projects))
	for id := range projects {
		projectIDs = append(projectIDs, id)
	}
	security, err := authentication.New(users, clusters, mcp, authentication.Config{PublicURL: settings.PublicURL, Development: settings.Development, ClusterID: settings.Cluster, Projects: projectIDs})
	if err != nil {
		return err
	}
	api := &controlplane.API{Store: store, GitHub: verifier, Organization: settings.Organization, Cluster: settings.Cluster, Projects: projects, RepositoryOwners: owners,
		Security: security, Development: settings.Development, WebhookSecret: []byte(webhookSecret), Domain: settings.Domain}
	if err := configureDatabases(ctx, api, settings, users); err != nil {
		return err
	}
	if len(settings.Cloud) > 0 {
		policies := map[string]cloud.Policy{}
		for _, policy := range settings.Cloud {
			project, ok := projects[policy.ProjectID]
			if !ok || policy.Scope != project.Scope || policy.ClusterID != settings.Cluster || policy.Validate() != nil || policies[policy.ProjectID].ProjectID != "" {
				return fmt.Errorf("invalid or duplicate operator cloud policy")
			}
			policies[policy.ProjectID] = policy
		}
		security.Cloud, err = clusteridentity.NewCloudRepository(store.Pool(), users, settings.Cluster)
		if err != nil {
			return err
		}
		if err := security.Cloud.Seed(ctx, settings.Cluster); err != nil {
			return err
		}
		api.Cloud = &cloudstore.Store{Pool: store.Pool(), Users: users, Policies: policies}
	}
	if settings.GitHubCredentialsFile != "" {
		api.GitHubApp, err = githubapp.New(store, users, githubapp.Config{CredentialsFile: settings.GitHubCredentialsFile, PublicURL: settings.PublicURL, Development: settings.Development, Projects: projects, RepositoryOwners: owners})
		if err != nil {
			return err
		}
		if err := api.GitHubApp.Initialize(ctx); err != nil {
			return err
		}
	}
	server := &http.Server{Addr: settings.Listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	group, active := errgroup.WithContext(ctx)
	group.Go(func() error {
		return (controlplane.Worker{Store: store, Publisher: publisher, Organization: settings.Organization}).Run(active)
	})
	group.Go(func() error { return serveHTTP(active, server) })
	if api.GitHubApp != nil {
		group.Go(func() error { return api.GitHubApp.Run(active) })
	}
	slog.Info("experimental Keldock control plane starting", "listen", settings.Listen, "organization", settings.Organization, "cluster", settings.Cluster)
	return group.Wait()
}

func serveHTTP(ctx context.Context, server *http.Server) error {
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServe() }()
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		if closeErr := server.Close(); closeErr != nil {
			return closeErr
		}
	}
	err := <-finished
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
