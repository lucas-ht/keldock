package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloudagent"
)

func cloudAgentCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := flags.String("config", "", "owner-only cloud agent configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var settings struct {
		APIURL         string         `json:"apiUrl"`
		CredentialFile string         `json:"credentialFile"`
		Kubeconfig     string         `json:"kubeconfig"`
		Context        string         `json:"context"`
		Development    bool           `json:"development"`
		Policies       []cloud.Policy `json:"policies"`
	}
	if err := readSettings(*config, &settings); err != nil {
		return err
	}
	if len(settings.Policies) == 0 || len(settings.Policies) > 32 {
		return fmt.Errorf("bounded local cloud policy required")
	}
	policies := map[string]cloud.Policy{}
	for _, policy := range settings.Policies {
		if err := policy.Validate(); err != nil {
			return err
		}
		if _, duplicate := policies[policy.ProjectID]; duplicate {
			return fmt.Errorf("duplicate cloud policy")
		}
		policies[policy.ProjectID] = policy
	}
	client, err := agent.CloudClientFromFile(settings.APIURL, settings.CredentialFile, settings.Development)
	if err != nil {
		return err
	}
	driver, err := cloudagent.New(settings.Kubeconfig, settings.Context)
	if err != nil {
		return err
	}
	return (&cloudagent.Agent{Client: client, Driver: driver, Policies: policies}).Run(ctx)
}
