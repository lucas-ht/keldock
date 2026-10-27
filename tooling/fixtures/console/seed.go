package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// These are synthetic fixture credentials, never a production default.
const password = "local-keldock-fixture-password"

func accounts(ctx context.Context, users *access.Repository, store *postgres.Store) (access.Principal, error) {
	var exists bool
	if err := store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM access_users WHERE organization_id=$1)`, users.Organization).Scan(&exists); err != nil {
		return access.Principal{}, err
	}
	if !exists {
		invitation, err := users.Bootstrap(ctx, "admin@keldock.test", "Local administrator")
		if err != nil {
			return access.Principal{}, err
		}
		hash, err := access.HashPassword(password)
		if err != nil {
			return access.Principal{}, err
		}
		if _, _, err := users.Activate(ctx, invitation.Token, hash); err != nil {
			return access.Principal{}, err
		}
	}
	account, err := users.PasswordAccount(ctx, "admin@keldock.test")
	if err != nil {
		return access.Principal{}, err
	}
	session, err := users.LoginSession(ctx, account.User)
	if err != nil {
		return access.Principal{}, err
	}
	principal, _, err := users.Session(ctx, session)
	if err != nil {
		return access.Principal{}, err
	}
	for _, role := range []string{"viewer", "operator"} {
		var present bool
		if err := store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM access_users WHERE organization_id=$1 AND email=$2)`, users.Organization, role+"@keldock.test").Scan(&present); err != nil {
			return principal, err
		}
		if present {
			continue
		}
		invitation, err := users.Invite(ctx, principal, role+"@keldock.test", "Local "+role, role, []string{"sample"})
		if err != nil {
			return principal, err
		}
		hash, err := access.HashPassword(password)
		if err != nil {
			return principal, err
		}
		if _, _, err := users.Activate(ctx, invitation.Token, hash); err != nil {
			return principal, err
		}
	}
	data, err := json.MarshalIndent(map[string]string{"email": "admin@keldock.test", "password": password, "note": "Synthetic local development credentials only"}, "", "  ")
	if err != nil {
		return principal, err
	}
	if err := os.WriteFile(".local/console-account.json", data, 0o600); err != nil {
		return principal, err
	}
	return principal, nil
}

func releases(ctx context.Context, store *postgres.Store, scope v1.Scope) error {
	data, err := os.ReadFile(".local/fixture-image.json")
	if err != nil {
		return err
	}
	var images struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal(data, &images); err != nil {
		return err
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	for _, pr := range []int64{0, 18, 19} {
		data, err := os.ReadFile("examples/multi-service/keldock.yaml")
		if err != nil {
			return err
		}
		definition, err := v1.DecodeApplication(data)
		if err != nil {
			return err
		}
		definition.Name = "sample"
		input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: scope.ProjectID, Definition: definition, Images: map[string]string{}, Source: v1.Source{RepositoryID: 42, OwnerID: 24, Repository: "example/sample", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main", Event: "push", WorkflowRef: "example/sample/.github/workflows/build.yaml@refs/heads/main", RunID: 1, RunAttempt: 1}, Provenance: json.RawMessage(`{}`)}
		for name := range definition.Components {
			input.Images[name] = images.Image
		}
		if pr > 0 {
			input.Source.PullRequest = pr
			input.Source.Event = "pull_request"
			input.Source.HeadCommit = strings.Repeat("b", 40)
			input.Source.Ref = "refs/pull/" + strconv.FormatInt(pr, 10) + "/merge"
			input.Source.WorkflowRef = "example/sample/.github/workflows/build.yaml@" + input.Source.Ref
		}
		environment := v1.EnvironmentID(scope, definition.Name, pr)
		var exists bool
		if err := store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE id=$1)`, environment).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		if pr == 18 {
			component := input.Definition.Components["migrate"]
			component.Env["FAIL_JOB"] = "true"
			input.Definition.Components["migrate"] = component
		}
		if pr > 0 {
			if _, err := store.RecordPullRequest(ctx, scope, "console-pr-"+strconv.FormatInt(pr, 10), strings.Repeat("c", 64), postgres.PullRequestUpdate{RepositoryID: 42, Number: pr, HeadCommit: input.Source.HeadCommit, HeadRepositoryID: 42, UpdatedAt: time.Now().UTC()}); err != nil {
				return err
			}
		}
		statement, err := provenance.NewStatement(input)
		if err != nil {
			return err
		}
		body, err := json.Marshal(statement)
		if err != nil {
			return err
		}
		envelope, err := provenance.Sign(body, "local-fixture", key)
		if err != nil {
			return err
		}
		input.Provenance, err = json.Marshal(envelope)
		if err != nil {
			return err
		}
		if err := provenance.VerifyRelease(input, map[string]ed25519.PublicKey{"local-fixture": key.Public().(ed25519.PublicKey)}); err != nil {
			return err
		}
		// Seeding is a fixture-only trust root; live GitHub OIDC is covered by runtime acceptance.
		if _, err := store.Admit(ctx, scope, "local-fixture-seed", fmt.Sprintf("console-initial-pr-%d", pr), "console-"+strings.TrimPrefix(environment, "env-")+".keldock.test", input); err != nil {
			return err
		}
	}
	return nil
}
