package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/cluster"
	"github.com/lucas-ht/keldock/internal/provenance"
	"github.com/lucas-ht/keldock/internal/recovery"
	"github.com/lucas-ht/keldock/internal/registryauth"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func recoveryCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: keldock recovery backup|verify|restore|restore-bundles|review|resume")
	}
	flags := flag.NewFlagSet("recovery", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "owner-only target control configuration")
	directory := flags.String("directory", "", "private snapshot directory")
	pulls := flags.String("registry-credentials-file", "", "private exact-repository image readers")
	kubeconfig := flags.String("kubeconfig", "", "exact surviving-cluster kubeconfig")
	kubeContext := flags.String("context", "", "exact surviving-cluster context")
	approved := flags.String("review-hash", "", "hash of the exact reviewed recovery inventory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if args[0] == "verify" {
		manifest, err := recovery.Verify(*directory)
		if err != nil {
			return err
		}
		fmt.Printf("Verified snapshot %s: %d files, %d immutable OCI references\n", manifest.ID, len(manifest.Files), len(manifest.Artifacts))
		return nil
	}
	var settings controlSettings
	if err := readSettings(*path, &settings); err != nil {
		return err
	}
	config, err := recoveryConfig(settings, *path, *pulls)
	if err != nil {
		return err
	}
	target, err := url.Parse(settings.DatabaseURL)
	if err != nil {
		return fmt.Errorf("invalid recovery database target")
	}
	fmt.Printf("Recovery target: organization=%s cluster=%s database=%s%s context=%s; no cloud account/backend changes\n", settings.Organization, settings.Cluster, target.Host, target.Path, *kubeContext)
	switch args[0] {
	case "backup":
		manifest, err := recovery.Backup(ctx, config, *directory)
		if err != nil {
			return err
		}
		fmt.Printf("Private snapshot %s saved to %s; encrypt and store it independently of this installation\n", manifest.ID, *directory)
		return nil
	case "restore":
		if err := recovery.Restore(ctx, config, *directory); err != nil {
			return err
		}
		fmt.Println("Database restored and fenced. Private installation files remain in the snapshot; review current authority before resuming or reenrolling agents.")
		return nil
	case "restore-bundles":
		return recovery.RestoreBundles(ctx, config, *directory)
	case "review", "resume":
		return recoveryReview(ctx, settings, *kubeconfig, *kubeContext, *approved, args[0] == "resume")
	default:
		return fmt.Errorf("unknown recovery command")
	}
}

func recoveryConfig(settings controlSettings, path, pulls string) (recovery.Config, error) {
	files := map[string]string{"control.json": path, "webhook.secret": settings.WebhookSecretFile}
	for name, file := range map[string]string{"github.json": settings.GitHubCredentialsFile, "registry-password": settings.RegistryPasswordFile, "registry-pulls.json": pulls} {
		if file != "" {
			if _, err := os.Lstat(file); os.IsNotExist(err) && name == "github.json" {
				continue
			}
			files[name] = file
		}
	}
	config := recovery.Config{Organization: settings.Organization, DatabaseURL: settings.DatabaseURL, Files: files, Registry: artifacts.RegistryConfig{PushRepository: settings.RegistryPush, PullRepository: settings.RegistryPull, PlainHTTP: settings.RegistryPlainHTTP, Username: settings.RegistryUsername}}
	var err error
	if settings.RegistryPasswordFile != "" {
		config.Registry.Password, err = credential(settings.RegistryPasswordFile, 1)
		if err != nil {
			return config, err
		}
	}
	config.Pulls, err = registryauth.Read(pulls, settings.Development)
	return config, err
}

func recoveryReview(ctx context.Context, settings controlSettings, kubeconfig, contextName, approved string, resume bool) error {
	s, err := postgres.Open(ctx, settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer s.Close()
	if fenced, err := s.RecoveryFenced(ctx, settings.Organization); err != nil {
		return err
	} else if !fenced {
		return fmt.Errorf("target is not a fenced recovery database")
	}
	if kubeconfig == "" || contextName == "" {
		return fmt.Errorf("exact surviving-cluster kubeconfig and context required")
	}
	driver, err := cluster.New(kubeconfig, contextName, settings.RegistryPull, settings.RegistryPlainHTTP)
	if err != nil {
		return err
	}
	journal, err := driver.Journal(ctx)
	if err != nil {
		return err
	}
	if err := driver.VerifyJournalCoverage(ctx, journal); err != nil {
		return err
	}
	data, err := driver.DatabaseJournal(ctx)
	if err != nil {
		return err
	}
	if err := s.ReviewRecovery(ctx, settings.Organization, journal); err != nil {
		return err
	}
	if err := s.ReviewDatabaseRecovery(ctx, settings.Organization, data); err != nil {
		return err
	}
	slices.SortFunc(journal, func(a, b cluster.JournalEntry) int { return strings.Compare(a.EnvironmentID, b.EnvironmentID) })
	slices.SortFunc(data, func(a, b cluster.DataCheckpoint) int {
		return strings.Compare(a.OrganizationID+"/"+a.ProjectID+"/"+a.ID, b.OrganizationID+"/"+b.ProjectID+"/"+b.ID)
	})
	inventory, err := s.RecoveryInventory(ctx, settings.Organization)
	if err != nil {
		return err
	}
	review := struct {
		Organization, Cluster, Context string
		Journal                        []cluster.JournalEntry
		Data                           []cluster.DataCheckpoint
		Inventory                      json.RawMessage
	}{settings.Organization, settings.Cluster, contextName, journal, data, inventory}
	hash, err := provenance.Hash(review)
	if err != nil {
		return err
	}
	if !resume {
		encoded, err := json.MarshalIndent(review, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		fmt.Println("Review hash:", hash)
		return nil
	}
	if approved != hash {
		return fmt.Errorf("recovery approval hash is absent or stale; review the exact current target first")
	}
	if err := s.ResumeRecovery(ctx, settings.Organization, journal); err != nil {
		return err
	}
	fmt.Println("Recovery fence released. Restart the server and explicitly enroll new agent identities; all recovered sessions, MCP grants and old agent credentials remain revoked.")
	return nil
}
