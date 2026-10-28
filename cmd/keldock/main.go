// Keldock is an experimental control-plane/agent runtime and CI provenance utility.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/buildinfo"
	"github.com/lucas-ht/keldock/internal/cluster"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "version" {
		fmt.Printf("keldock %s (%s)\n", buildinfo.Version, buildinfo.Revision)
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: keldock serve|agent|database-agent|enroll|bootstrap|recover|recovery|installation|inspect|validate|prepare|attest|keygen|cloud|version")
	}
	if args[0] == "init-files" {
		return privateFilesCommand(args[1:])
	}
	if args[0] == "process-health" {
		return processHealthCommand(args[1:])
	}
	if args[0] == "installation" {
		return installationCommand(ctx, args[1:])
	}
	if args[0] == "cloud" {
		return cloudCommand(args[1:])
	}
	if args[0] == "recovery" {
		return recoveryCommand(ctx, args[1:])
	}
	if args[0] == "cloud-agent" {
		return cloudAgentCommand(ctx, args[1:])
	}
	if args[0] == "database-agent" {
		return databaseAgentCommand(ctx, args[1:])
	}
	if args[0] == "prepare" {
		return prepareCommand(args[1:])
	}
	if args[0] == "attest" || args[0] == "keygen" {
		return provenanceCommand(args)
	}
	if args[0] == "bootstrap" || args[0] == "recover" || args[0] == "enroll" {
		return identityCommand(ctx, args)
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configuration := flags.String("config", "", "private runtime configuration file")
	definition := flags.String("file", "keldock.yaml", "application definition")
	migrate := flags.Bool("migrate", false, "explicitly apply control-plane database migrations")
	waitEnrollment := flags.Bool("wait-for-enrollment", false, "wait for an operator to enroll this process")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "validate":
		data, err := os.ReadFile(*definition)
		if err != nil {
			return err
		}
		app, err := v1.DecodeApplication(data)
		if err != nil {
			return err
		}
		fmt.Printf("Valid %s application %s with %d components\n", app.APIVersion, app.Name, len(app.Components))
		return nil
	case "serve":
		var settings controlSettings
		if err := readSettings(*configuration, &settings); err != nil {
			return err
		}
		return serve(ctx, settings, *migrate)
	case "agent", "inspect":
		var settings agentSettings
		if err := readSettings(*configuration, &settings); err != nil {
			return err
		}
		if settings.RegistryPlainHTTP && !settings.Development {
			return fmt.Errorf("plain HTTP registry is a development-only profile")
		}
		driver, err := cluster.New(settings.Kubeconfig, settings.Context, settings.RegistryPull, settings.RegistryPlainHTTP)
		if err != nil {
			return err
		}
		driver.CredentialsFile = settings.RegistryCredentialsFile
		dependencies, err := driver.Inspect(ctx)
		if err != nil {
			return err
		}
		if args[0] == "inspect" {
			return json.NewEncoder(os.Stdout).Encode(dependencies)
		}
		return runEnrolled(ctx, settings.CredentialFile, settings.HealthFile, *waitEnrollment, func() error {
			client, err := agent.ClientFromFile(settings.APIURL, settings.CredentialFile, settings.Development)
			if err != nil {
				return err
			}
			return (agent.Agent{Client: client, Driver: driver, HealthFile: settings.HealthFile}).Run(ctx)
		})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
