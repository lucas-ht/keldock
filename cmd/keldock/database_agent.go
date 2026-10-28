package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/databaseagent"
)

func databaseAgentCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("database-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := flags.String("config", "", "owner-only database agent configuration")
	waitEnrollment := flags.Bool("wait-for-enrollment", false, "wait for explicit database provisioner enrollment")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var settings struct {
		agentSettings
		Policies []database.Policy `json:"policies"`
	}
	if err := readSettings(*config, &settings); err != nil {
		return err
	}
	policies := map[string]database.Policy{}
	for _, p := range settings.Policies {
		if p.Validate() != nil || policies[p.ProjectID].ProjectID != "" {
			return fmt.Errorf("invalid independent database policy")
		}
		policies[p.ProjectID] = p
	}
	if len(policies) == 0 {
		return fmt.Errorf("database delegation required")
	}
	driver, err := databaseagent.New(settings.Kubeconfig, settings.Context)
	if err != nil {
		return err
	}
	if err := driver.Inspect(ctx); err != nil {
		return err
	}
	return runEnrolled(ctx, settings.CredentialFile, settings.HealthFile, *waitEnrollment, func() error {
		client, err := agent.DatabaseClientFromFile(settings.APIURL, settings.CredentialFile, settings.Development)
		if err != nil {
			return err
		}
		return (databaseagent.Agent{Client: client, Driver: driver, Policies: policies, HealthFile: settings.HealthFile}).Run(ctx)
	})
}
