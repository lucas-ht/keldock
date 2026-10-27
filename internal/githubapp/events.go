package githubapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
	"github.com/lucas-ht/keldock/internal/githubapp/store"
)

type eventEnvelope struct {
	Action       string `json:"action"`
	Number       int64  `json:"number"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository client.Repository   `json:"repository"`
	Removed    []client.Repository `json:"repositories_removed"`
}

// Receive verifies the exact bytes before acknowledging durable work or fencing revocation.
func (service *Service) Receive(ctx context.Context, id, name, signature string, payload []byte) (bool, error) {
	_, app, err := service.active(ctx)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(signature, "sha256=") || len(payload) > 1024*1024 {
		return false, access.ErrDenied
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false, access.ErrDenied
	}
	mac := hmac.New(sha256.New, []byte(app.WebhookSecret))
	if _, err := mac.Write(payload); err != nil {
		return false, err
	}
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return false, access.ErrDenied
	}
	var envelope eventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return false, access.ErrInvalid
	}
	revocations := store.Revocations{InstallationID: envelope.Installation.ID}
	if name == "installation" && (envelope.Action == "deleted" || envelope.Action == "suspend") {
		revocations.Installation = true
	}
	if name == "installation_repositories" && envelope.Action == "removed" {
		for _, repository := range envelope.Removed {
			if repository.ID > 0 {
				revocations.RepositoryIDs = append(revocations.RepositoryIDs, repository.ID)
			}
		}
	}
	return service.Repository.Receive(ctx, id, name, payload, revocations)
}

// ProcessEvent handles one leased delivery; duplicate processing uses authoritative snapshots.
func (service *Service) ProcessEvent(ctx context.Context) (bool, error) {
	event, err := service.Repository.ClaimEvent(ctx)
	if err != nil || event == nil {
		return false, err
	}
	work, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var envelope eventEnvelope
	err = json.Unmarshal(event.Payload, &envelope)
	if err == nil {
		switch event.Name {
		case "installation", "installation_repositories", "repository":
			err = service.Sync(work)
		case "pull_request":
			project := ""
			for id, item := range service.Config.Projects {
				if item.RepositoryID == envelope.Repository.ID && service.Config.RepositoryOwners[item.RepositoryID] == envelope.Repository.Owner.ID {
					project = id
				}
			}
			if project != "" && envelope.Number > 0 {
				grant, denied := service.Repository.Grant(work, envelope.Repository.ID)
				if denied == nil {
					api, _, failure := service.active(work)
					err = failure
					if err == nil {
						err = service.recoverPullRequests(work, api, grant)
					}
				} else if !errors.Is(denied, access.ErrDenied) {
					err = denied
				}
			}
		}
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	retry := 5 * time.Second * time.Duration(1<<min(event.Attempt, 6))
	var upstream *client.Error
	if errors.As(err, &upstream) {
		retry = max(retry, upstream.RetryAfter)
	}
	if finish := service.Repository.FinishEvent(ctx, *event, err == nil, retry); finish != nil {
		return true, finish
	}
	return true, err
}

// Run recovers webhook work and periodically repairs missed GitHub selection/PR state.
func (service *Service) Run(ctx context.Context) error {
	var nextSync time.Time
	for ctx.Err() == nil {
		if time.Now().After(nextSync) {
			work, cancel := context.WithTimeout(ctx, 45*time.Second)
			err := service.Sync(work)
			cancel()
			if err != nil && !errors.Is(err, access.ErrDenied) {
				slog.Warn("GitHub synchronization retry", "error", err)
			}
			nextSync = time.Now().Add(time.Minute)
		}
		didWork, err := service.ProcessEvent(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("GitHub webhook retry", "error", err)
		}
		if didWork && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
	return nil
}
