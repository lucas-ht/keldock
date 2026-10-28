package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/lucas-ht/keldock/internal/admission"
	"github.com/lucas-ht/keldock/internal/artifacts"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/registryauth"
)

type releaseManifest struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Runtime  string `json:"runtimeImage"`
	Console  string `json:"consoleImage"`
}

func installationInit(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("directory", "", "new private installation directory")
	releaseFile := flags.String("release-file", "", "reviewed release image manifest")
	name := flags.String("name", "keldock", "Helm release and cluster identifier")
	namespace := flags.String("namespace", "keldock", "installation namespace")
	organization := flags.String("organization", "default", "installation organization identifier")
	publicURL := flags.String("public-url", "", "canonical reachable HTTPS origin")
	domain := flags.String("domain", "", "operator-controlled application domain")
	storage := flags.String("storage-class", "", "tested dynamic StorageClass")
	tlsSecret := flags.String("tls-secret", "keldock-tls", "existing TLS Secret in installation namespace")
	development := flags.Bool("development", false, "permit explicit loopback HTTP for local acceptance")
	databaseURLFile := flags.String("database-url-file", "", "owner-only supplied PostgreSQL 18 connection URI")
	registryPush := flags.String("registry-push", "", "supplied compatible bundle registry repository")
	registryPull := flags.String("registry-pull", "", "same repository as seen by the cluster")
	registryUser := flags.String("registry-user", "", "scoped bundle publisher username")
	registryPassword := flags.String("registry-password-file", "", "owner-only scoped publisher password")
	registryReaders := flags.String("registry-readers-file", "", "owner-only scoped bundle/workload reader configuration")
	registryHTTP := flags.Bool("registry-plain-http", false, "explicit local supplied registry transport")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !v1.ValidName(*name) || !v1.ValidName(*namespace) || !v1.ValidName(*organization) || !v1.ValidName(*tlsSecret) || *storage == "" || *domain == "" || *directory == "" {
		return fmt.Errorf("installation identifiers, domain, storage class and directory are required")
	}
	parsed, err := url.Parse(*publicURL)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("canonical HTTPS origin required")
	}
	local := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if parsed.Scheme != "https" && (!*development || !local || parsed.Scheme != "http") {
		return fmt.Errorf("installation requires HTTPS; development HTTP is loopback-only")
	}
	data, err := boundedFile(*releaseFile, 65536)
	if err != nil {
		return err
	}
	var release releaseManifest
	if err := v1.DecodeJSON(data, &release); err != nil {
		return err
	}
	for _, image := range []string{release.Runtime, release.Console} {
		if _, _, err := artifacts.SplitReference(image); err != nil {
			return fmt.Errorf("release manifest requires immutable runtime and console images")
		}
	}
	if release.Version == "" || release.Revision == "" {
		return fmt.Errorf("release identity is required")
	}
	if err := os.Mkdir(*directory, 0o700); err != nil {
		return fmt.Errorf("installation destination must be a new directory")
	}
	if (*registryPush == "") != (*registryPull == "") || (*registryHTTP && !*development) {
		return fmt.Errorf("supplied registry requires both repository aliases; HTTP requires explicit development")
	}
	password := rand.Text() + rand.Text()
	webhook := rand.Text() + rand.Text()
	dsn := "postgres://keldock:" + password + "@" + *name + "-database:5432/keldock?sslmode=disable"
	if *databaseURLFile != "" {
		dsn, err = credential(*databaseURLFile, 1)
		if err != nil {
			return err
		}
		db, err := url.Parse(dsn)
		if err != nil || db.User == nil || (db.Scheme != "postgres" && db.Scheme != "postgresql") || db.Hostname() == "" || (!*development && db.Query().Get("sslmode") != "verify-full") {
			return fmt.Errorf("supplied PostgreSQL requires a connection URI with sslmode=verify-full outside explicit development")
		}
	}
	// The embedded registry uses private cluster transport in the development alpha.
	// Public auth still requires canonical HTTPS and secure cookies.
	settings := controlSettings{DatabaseURL: dsn, Listen: ":3801", Organization: *organization, Cluster: *name, Domain: *domain, RegistryPush: *name + "-registry." + *namespace + ".svc.cluster.local:5000/keldock/bundles", RegistryPull: *name + "-registry." + *namespace + ".svc.cluster.local:5000/keldock/bundles", RegistryPlainHTTP: true, PublicURL: parsed.Scheme + "://" + parsed.Host, WebhookSecretFile: "/runtime/private/webhook.secret", GitHubIssuer: "https://token.actions.githubusercontent.com", GitHubAudience: parsed.Scheme + "://" + parsed.Host, GitHubCredentialsFile: "/state/private/github.json", Development: true, Projects: []repositorySettings{}}
	agent := agentSettings{APIURL: settings.PublicURL, CredentialFile: "/state/private/identity.json", RegistryPull: settings.RegistryPull, RegistryPlainHTTP: true, Development: true, HealthFile: "/tmp/keldock-agent-health"}
	values := map[string]any{"images": map[string]string{"runtime": release.Runtime, "console": release.Console}, "installation": map[string]string{"existingSecret": *name + "-installation", "publicURL": settings.PublicURL}, "storageClass": *storage, "database": map[string]bool{"enabled": *databaseURLFile == ""}, "registry": map[string]bool{"enabled": true}, "databases": map[string]bool{"enabled": false}, "ingress": map[string]any{"enabled": parsed.Scheme == "https", "className": "traefik", "host": parsed.Hostname(), "tlsSecret": *tlsSecret}}
	if *registryPush != "" {
		settings.RegistryPush = *registryPush
		settings.RegistryPull = *registryPull
		settings.RegistryUsername = *registryUser
		settings.RegistryPlainHTTP = *registryHTTP
		settings.Development = *development
		password := ""
		if *registryPassword != "" {
			password, err = credential(*registryPassword, 1)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(*directory, "registry-password"), []byte(password), 0o600); err != nil {
				return err
			}
			settings.RegistryPasswordFile = "/runtime/private/registry-password"
		}
		if _, err := artifacts.NewPublisher(artifacts.RegistryConfig{PushRepository: *registryPush, PullRepository: *registryPull, PlainHTTP: *registryHTTP, Username: *registryUser, Password: password}); err != nil {
			return err
		}
		agent.RegistryPull = *registryPull
		agent.RegistryPlainHTTP = *registryHTTP
		agent.Development = *development
		values["registry"] = map[string]bool{"enabled": false}
		if *registryPassword != "" {
			values["publisherCredential"] = true
		}
	}
	if *registryReaders != "" {
		readers, err := registryauth.Read(*registryReaders, settings.Development)
		if err != nil {
			return err
		}
		if err := writePrivateJSON(filepath.Join(*directory, "registry-pulls.json"), readers); err != nil {
			return err
		}
		agent.RegistryCredentialsFile = "/runtime/private/registry-pulls.json"
		values["registryReaders"] = true
	}
	for name, value := range map[string]any{"control.json": settings, "agent.json": agent, "values.json": values} {
		if err := writePrivateJSON(filepath.Join(*directory, name), value); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{"webhook.secret": webhook, "postgres-password": password, "postgres-admin-password": rand.Text() + rand.Text()} {
		if err := os.WriteFile(filepath.Join(*directory, name), []byte(value), 0o600); err != nil {
			return err
		}
	}
	if err := writeInstallationSecret(*directory, *name+"-installation", *namespace); err != nil {
		return err
	}
	if _, err := admission.NewGitHub(ctx, settings.GitHubIssuer, settings.GitHubAudience, nil, false); err != nil {
		return err
	}
	fmt.Println("Private installation inputs prepared in", *directory)
	fmt.Printf("Target: organization=%s cluster=%s namespace=%s publicURL=%s; review and run preflight before installing\n", settings.Organization, settings.Cluster, *namespace, settings.PublicURL)
	return nil
}

func writePrivateJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func writeInstallationSecret(directory, name, namespace string) error {
	values := map[string][]byte{}
	for _, file := range []string{"control.json", "agent.json", "database-agent.json", "webhook.secret", "postgres-password", "postgres-admin-password", "registry-password", "registry-pulls.json"} {
		data, err := os.ReadFile(filepath.Join(directory, file))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		values[file] = data
	}
	return writePrivateJSON(filepath.Join(directory, "installation-secret.json"), map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]string{"name": name, "namespace": namespace}, "type": "Opaque", "data": values})
}

func addDatabasePolicy(settings *controlSettings, project string, storageClass string) {
	settings.Databases = append(settings.Databases, database.Policy{Scope: v1.Scope{OrganizationID: settings.Organization, ProjectID: project}, ClusterID: settings.Cluster, Limit: 4, Profiles: map[string]database.Profile{"development": {StorageClass: storageClass, StorageSize: "1Gi"}}})
}
