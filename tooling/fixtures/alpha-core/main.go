// Package main seeds only the named disposable core fixture with synthetic admission.
// GitHub/OIDC/provenance admission is covered separately; this tests packaged dispatch.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	var config struct {
		DatabaseURL  string `json:"databaseUrl"`
		Organization string `json:"organizationId"`
		Cluster      string `json:"clusterId"`
		Domain       string `json:"domain"`
	}
	if err := read(".local/m6/core/installation/control.json", &config); err != nil {
		return err
	}
	db, err := url.Parse(config.DatabaseURL)
	if err != nil || config.Organization != "core-fixture" || config.Cluster != "core" || db.Host != "core-database:5432" {
		return fmt.Errorf("refusing admission outside exact disposable core fixture")
	}
	db.Host = "127.0.0.1:65432"
	s, err := postgres.Open(ctx, db.String())
	if err != nil {
		return err
	}
	defer s.Close()
	data, err := os.ReadFile("examples/multi-service/keldock.yaml")
	if err != nil {
		return err
	}
	app, err := v1.DecodeApplication(data)
	if err != nil {
		return err
	}
	var images struct {
		Image string `json:"image"`
	}
	if err := read(".local/fixture-image.json", &images); err != nil {
		return err
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: "sample", Definition: app, Images: map[string]string{}, Source: v1.Source{RepositoryID: 42, OwnerID: 24, Repository: "fixture/core", Commit: strings.Repeat("c", 40), Ref: "refs/heads/main", Event: "push", WorkflowRef: "fixture/core/.github/workflows/build.yaml@refs/heads/main", RunID: 1, RunAttempt: 1}, Provenance: json.RawMessage(`{}`)}
	if len(os.Args) > 1 && os.Args[1] == "--refresh" {
		input.Source.RunID = 2
	}
	for name := range app.Components {
		input.Images[name] = images.Image
	}
	scope := v1.Scope{OrganizationID: config.Organization, ProjectID: input.ProjectID}
	receipt, err := s.AdmitToDomain(ctx, scope, "synthetic-local-fixture", fmt.Sprintf("packaged-core-release-%d", input.Source.RunID), config.Domain, input)
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		op, err := s.Operation(ctx, scope, receipt.OperationID)
		if err != nil {
			return err
		}
		if op.State == "failed" {
			return fmt.Errorf("packaged core rollout failed: %s", op.Error)
		}
		if op.State == "ready" {
			current, err := s.CurrentDesired(ctx, scope.OrganizationID, config.Cluster, receipt.EnvironmentID)
			if err != nil {
				return err
			}
			if len(current.Selection.Databases) != 0 || len(current.Definition.Resources) != 0 {
				return fmt.Errorf("core fixture unexpectedly required an application database")
			}
			data, err := json.MarshalIndent(map[string]any{"receipt": receipt, "placement": current.Selection, "images": current.Images}, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(".local/m6/core/application-evidence.json", data, 0o600); err != nil {
				return err
			}
			fmt.Println("Packaged API/worker and enrolled registrar delivered all five components using supplied immutable synthetic CI images, without CNPG or cloud controllers.")
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return ctx.Err()
}
func read(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
