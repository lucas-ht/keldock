package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func installationSecretCommand(args []string) error {
	flags := flag.NewFlagSet("render-secret", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("directory", "", "private installation input directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var settings controlSettings
	if err := readSettings(filepath.Join(*directory, "control.json"), &settings); err != nil {
		return err
	}
	if !v1.ValidName(settings.Organization) || !v1.ValidName(settings.Cluster) {
		return fmt.Errorf("invalid installation authority")
	}
	for _, policy := range settings.Databases {
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	data, err := boundedFile(filepath.Join(*directory, "installation-secret.json"), 1024*1024)
	if err != nil {
		return err
	}
	var previous struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &previous); err != nil {
		return err
	}
	if previous.Kind != "Secret" || !v1.ValidName(previous.Metadata.Name) || !v1.ValidName(previous.Metadata.Namespace) {
		return fmt.Errorf("invalid prepared Secret target")
	}
	if err := writeInstallationSecret(*directory, previous.Metadata.Name, previous.Metadata.Namespace); err != nil {
		return err
	}
	fmt.Printf("Prepared private Secret %s/%s for review. No cluster resources changed.\n", previous.Metadata.Namespace, previous.Metadata.Name)
	return nil
}
