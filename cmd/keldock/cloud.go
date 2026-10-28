package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lucas-ht/keldock/internal/cloud/bootstrap"
)

// cloudCommand prepares an offline, immutable review bundle; planning never applies AWS changes.
func cloudCommand(args []string) error {
	if len(args) == 0 || args[0] != "plan" {
		return fmt.Errorf("usage: keldock cloud plan --config <private-spec.json> --out <new-directory>")
	}
	flags := flag.NewFlagSet("cloud plan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configuration := flags.String("config", "", "owner-only bootstrap specification")
	directory := flags.String("out", "", "new owner-only review directory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configuration == "" || *directory == "" {
		return fmt.Errorf("config and a new output directory are required")
	}
	var spec bootstrap.Spec
	if err := readSettings(*configuration, &spec); err != nil {
		return err
	}
	plan, err := bootstrap.Build(spec)
	if err != nil {
		return err
	}
	fingerprint, err := plan.Fingerprint()
	if err != nil {
		return err
	}
	if err := os.Mkdir(*directory, 0o700); err != nil {
		return fmt.Errorf("create new review directory: %w", err)
	}
	for name, value := range map[string]any{"plan.json": plan, "foundation.json": plan.Foundation, "identity.json": plan.Identity, "policy-example.json": spec.Policy("ZREPLACEWITHCHILDZONE")} {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*directory, name), append(data, '\n'), 0o600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(*directory, "sha256.txt"), []byte(fingerprint+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("Prepared AWS review bundle %s\nAccount %s; region %s; child domain %s\nSHA256 %s\nNo AWS changes applied.\n", *directory, spec.Account, spec.Region, spec.Domain, fingerprint)
	return nil
}
