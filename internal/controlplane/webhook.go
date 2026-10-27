package controlplane

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

type pullRequestEvent struct {
	Action     string `json:"action"`
	Number     int64  `json:"number"`
	Repository struct {
		ID    int64 `json:"id"`
		Owner struct {
			ID int64 `json:"id"`
		} `json:"owner"`
	} `json:"repository"`
	PullRequest struct {
		Number    int64     `json:"number"`
		State     string    `json:"state"`
		UpdatedAt time.Time `json:"updated_at"`
		Head      struct {
			SHA        string `json:"sha"`
			Repository struct {
				ID int64 `json:"id"`
			} `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
}

func (api *API) webhook(w http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, 1024*1024))
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_request", "Invalid webhook body")
		return
	}
	header := request.Header.Get("X-Hub-Signature-256")
	if api.GitHubApp != nil {
		changed, err := api.GitHubApp.Receive(request.Context(), request.Header.Get("X-GitHub-Delivery"), request.Header.Get("X-GitHub-Event"), header, body)
		if err != nil {
			failure(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"state": "queued", "changed": changed})
		return
	}
	if !strings.HasPrefix(header, "sha256=") {
		failure(w, postgres.ErrDenied)
		return
	}
	signature, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	mac := hmac.New(sha256.New, api.WebhookSecret)
	if _, err := mac.Write(body); err != nil {
		failure(w, err)
		return
	}
	if len(api.WebhookSecret) < 32 || err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		failure(w, postgres.ErrDenied)
		return
	}
	if request.Header.Get("X-GitHub-Event") != "pull_request" {
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "ignored"})
		return
	}
	var event pullRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		problem(w, http.StatusBadRequest, "invalid_request", "Invalid GitHub event")
		return
	}
	if !slices.Contains([]string{"opened", "reopened", "synchronize", "closed"}, event.Action) {
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "ignored"})
		return
	}
	owner, configured := api.RepositoryOwners[event.Repository.ID]
	if !configured || event.Number != event.PullRequest.Number || event.Number <= 0 || event.Repository.Owner.ID != owner || (event.Action == "closed") != (event.PullRequest.State == "closed") || (event.PullRequest.State != "closed" && event.PullRequest.State != "open") {
		failure(w, postgres.ErrDenied)
		return
	}
	for _, project := range api.Projects {
		if project.RepositoryID != event.Repository.ID || project.OrganizationID != api.Organization {
			continue
		}
		hash := sha256.Sum256(body)
		changed, err := api.Store.RecordPullRequest(request.Context(), project.Scope, request.Header.Get("X-GitHub-Delivery"), hex.EncodeToString(hash[:]), postgres.PullRequestUpdate{
			RepositoryID: event.Repository.ID, Number: event.Number, HeadCommit: event.PullRequest.Head.SHA, HeadRepositoryID: event.PullRequest.Head.Repository.ID, Closed: event.PullRequest.State == "closed", UpdatedAt: event.PullRequest.UpdatedAt})
		if err != nil {
			failure(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"state": "recorded", "changed": changed})
		return
	}
	failure(w, postgres.ErrDenied)
}
