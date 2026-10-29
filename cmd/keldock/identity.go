package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/agent"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Identity recovery uses local database authority; no public bootstrap endpoint exists.
func identityCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := flags.String("config", "", "owner-only installation configuration")
	email := flags.String("email", "", "account email")
	name := flags.String("name", "Administrator", "initial administrator name")
	output := flags.String("output", "", "new owner-only invitation file")
	passwordFile := flags.String("password-file", "", "owner-only recovery password file")
	tokenFile := flags.String("token-file", "", "one-time enrollment token file")
	cluster := flags.String("cluster", "", "configured cluster identifier")
	cloud := flags.Bool("cloud", false, "enroll the separate optional cloud agent")
	database := flags.Bool("database", false, "enroll the separate optional PostgreSQL agent")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if args[0] == "enroll" {
		settings, err := readEnrollmentSettings(*config, *cloud, *database)
		if err != nil {
			return err
		}
		token, err := secret(*tokenFile)
		if err != nil {
			return err
		}
		enroll := agent.Enroll
		if *cloud {
			enroll = agent.EnrollCloud
		}
		if *database {
			enroll = agent.EnrollDatabase
		}
		if err := enroll(ctx, settings.APIURL, *cluster, token, settings.CredentialFile, settings.Development); err != nil {
			return err
		}
		fmt.Println("Agent enrolled; credential saved to", settings.CredentialFile)
		return nil
	}

	var settings controlSettings
	if err := readSettings(*config, &settings); err != nil {
		return err
	}
	store, err := postgres.Open(ctx, settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	repository, err := access.NewRepository(store.Pool(), settings.Organization)
	if err != nil {
		return err
	}
	if args[0] == "recover" {
		password, err := credential(*passwordFile, 12)
		if err != nil {
			return err
		}
		hash, err := access.HashPassword(password)
		if err != nil {
			return err
		}
		if err := repository.Recover(ctx, *email, hash); err != nil {
			return err
		}
		fmt.Println("Account recovered; previous sessions and MCP connections revoked")
		return nil
	}
	if *output == "" {
		return fmt.Errorf("--output is required; invitation secrets are never printed")
	}
	if _, err := os.Lstat(*output); !os.IsNotExist(err) {
		return fmt.Errorf("invitation destination must not exist")
	}
	grant, err := repository.Bootstrap(ctx, *email, *name)
	if err != nil {
		return err
	}
	value := map[string]any{"activationUrl": settings.PublicURL + "/activate#token=" + url.QueryEscape(grant.Token), "expiresAt": grant.ExpiresAt, "email": grant.User.Email}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	fmt.Println("One-time account invitation saved to", *output)
	return nil
}
