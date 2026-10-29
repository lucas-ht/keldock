package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lucas-ht/keldock/internal/admission"
	"github.com/lucas-ht/keldock/internal/database"
)

func installationProject(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("project", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("directory", "", "existing private installation input directory")
	file := flags.String("file", "", "private repository policy JSON with public provenance keys")
	postgresql := flags.Bool("postgresql", false, "explicitly delegate the tested PostgreSQL development profile")
	if err := flags.Parse(args); err != nil {
		return err
	}
	path := filepath.Join(*directory, "control.json")
	var settings controlSettings
	if err := readSettings(path, &settings); err != nil {
		return err
	}
	var project repositorySettings
	if err := readSettings(*file, &project); err != nil {
		return err
	}
	requested, err := json.Marshal(project)
	if err != nil {
		return err
	}
	newProject := true
	for _, existing := range settings.Projects {
		if existing.ID == project.ID || existing.RepositoryID == project.RepositoryID {
			configured, err := json.Marshal(existing)
			if err != nil {
				return err
			}
			if !*postgresql || !bytes.Equal(configured, requested) {
				return fmt.Errorf("project or repository already configured; PostgreSQL delegation requires its unchanged repository policy")
			}
			newProject = false
		}
	}
	if *postgresql {
		for _, policy := range settings.Databases {
			if policy.ProjectID == project.ID {
				return fmt.Errorf("PostgreSQL already configured for this project; review its existing policy explicitly")
			}
		}
	}
	policy, err := settings.policy(project)
	if err != nil {
		return err
	}
	if _, err := admission.NewGitHub(ctx, settings.GitHubIssuer, settings.GitHubAudience, []admission.RepositoryPolicy{policy}, false); err != nil {
		return err
	}
	var values map[string]any
	data, err := os.ReadFile(filepath.Join(*directory, "values.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if newProject {
		settings.Projects = append(settings.Projects, project)
	}
	if *postgresql {
		storage, ok := values["storageClass"].(string)
		if !ok || storage == "" {
			return fmt.Errorf("database storage class is absent")
		}
		addDatabasePolicy(&settings, project.ID, storage)
		values["databases"] = map[string]bool{"enabled": true}
		var agent agentSettings
		if err := readSettings(filepath.Join(*directory, "agent.json"), &agent); err != nil {
			return err
		}
		databaseAgent := struct {
			agentSettings
			Policies []database.Policy `json:"policies"`
		}{agent, settings.Databases}
		if err := writePrivateJSON(filepath.Join(*directory, "database-agent.json"), databaseAgent); err != nil {
			return err
		}
	}
	if err := writePrivateJSON(path, settings); err != nil {
		return err
	}
	if err := writePrivateJSON(filepath.Join(*directory, "values.json"), values); err != nil {
		return err
	}
	var secret struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	data, err = os.ReadFile(filepath.Join(*directory, "installation-secret.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &secret); err != nil {
		return err
	}
	if err := writeInstallationSecret(*directory, secret.Metadata.Name, secret.Metadata.Namespace); err != nil {
		return err
	}
	fmt.Printf("Prepared project %s for repository ID %d. Review the private Secret update and restart the control plane after applying it.\n", project.ID, project.RepositoryID)
	return nil
}

func installationCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: keldock installation init|project|preflight")
	}
	switch args[0] {
	case "init":
		return installationInit(ctx, args[1:])
	case "project":
		return installationProject(ctx, args[1:])
	case "preflight":
		return installationPreflight(ctx, args[1:])
	case "render-secret":
		return installationSecretCommand(args[1:])
	default:
		return fmt.Errorf("unknown installation command")
	}
}
