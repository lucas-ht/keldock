package client

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type remoteCheck struct {
	ID         int64  `json:"id"`
	ExternalID string `json:"external_id"`
	App        App    `json:"app"`
}
type remoteDeployment struct {
	ID      int64          `json:"id"`
	Payload map[string]any `json:"payload"`
}
type remoteComment struct {
	ID   int64   `json:"id"`
	Body string  `json:"body"`
	User Account `json:"user"`
}

// Check recovers a lost create response through App-owned external identity before updating.
func (client *Client) Check(ctx context.Context, token Token, repo, head, marker string, id int64, input map[string]any) (int64, error) {
	path, err := RepositoryPath(repo)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		for page := 1; page <= 10; page++ {
			var result struct {
				Items []remoteCheck `json:"check_runs"`
			}
			err := client.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("%s/commits/%s/check-runs?check_name=Keldock+delivery&filter=all&per_page=100&page=%d", path, url.PathEscape(head), page), nil, &result)
			if err != nil {
				return 0, err
			}
			for _, check := range result.Items {
				if check.ExternalID == marker && check.App.ID == client.credentials.ID {
					id = check.ID
					break
				}
			}
			if id > 0 || len(result.Items) < 100 {
				break
			}
			if page == 10 {
				return 0, fmt.Errorf("GitHub check recovery exceeds supported bound")
			}
		}
	}
	if id == 0 {
		input["name"], input["head_sha"], input["external_id"] = "Keldock delivery", head, marker
		var created remoteCheck
		if err := client.RepositoryRequest(ctx, token, "POST", path+"/check-runs", input, &created); err != nil {
			return 0, err
		}
		if created.ID <= 0 {
			return 0, fmt.Errorf("invalid GitHub check identity")
		}
		return created.ID, nil
	}
	err = client.RepositoryRequest(ctx, token, "PATCH", fmt.Sprintf("%s/check-runs/%d", path, id), input, nil)
	return id, err
}

// Deployment recovers an operation-marked deployment without asking GitHub to merge or build.
func (client *Client) Deployment(ctx context.Context, token Token, repo, commit, environment, marker string, preview bool) (int64, error) {
	path, err := RepositoryPath(repo)
	if err != nil {
		return 0, err
	}
	for page := 1; page <= 10; page++ {
		var items []remoteDeployment
		err := client.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("%s/deployments?sha=%s&environment=%s&task=keldock%%2Fdeploy&per_page=100&page=%d", path, url.QueryEscape(commit), url.QueryEscape(environment), page), nil, &items)
		if err != nil {
			return 0, err
		}
		for _, item := range items {
			if item.Payload["keldock_operation"] == marker {
				return item.ID, nil
			}
		}
		if len(items) < 100 {
			break
		}
		if page == 10 {
			return 0, fmt.Errorf("GitHub deployment recovery exceeds supported bound")
		}
	}
	var created remoteDeployment
	input := map[string]any{"ref": commit, "task": "keldock/deploy", "auto_merge": false, "required_contexts": []string{}, "environment": environment, "transient_environment": preview, "production_environment": false, "payload": map[string]string{"keldock_operation": marker}}
	if err := client.RepositoryRequest(ctx, token, "POST", path+"/deployments", input, &created); err != nil {
		return 0, err
	}
	if created.ID <= 0 {
		return 0, fmt.Errorf("invalid GitHub deployment identity")
	}
	return created.ID, nil
}

// DeploymentStatus avoids duplicating an already successful remote status after a lost response.
func (client *Client) DeploymentStatus(ctx context.Context, token Token, repo string, id int64, state, title, details, marker string) error {
	path, err := RepositoryPath(repo)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/deployments/%d/statuses", path, id)
	var latest []struct {
		State       string `json:"state"`
		Description string `json:"description"`
	}
	if err := client.RepositoryRequest(ctx, token, "GET", endpoint+"?per_page=1", nil, &latest); err != nil {
		return err
	}
	description := title + " [" + marker[len(marker)-12:] + "]"
	if len(description) > 140 {
		description = description[:140]
	}
	if len(latest) > 0 && latest[0].State == state && latest[0].Description == description {
		return nil
	}
	return client.RepositoryRequest(ctx, token, "POST", endpoint, map[string]any{"state": state, "description": description, "log_url": details, "environment_url": details, "auto_inactive": false}, nil)
}

// Comment recovers and updates only this App's marked comment in the selected PR.
func (client *Client) Comment(ctx context.Context, token Token, repo string, number, id int64, marker, body string) (int64, error) {
	path, err := RepositoryPath(repo)
	if err != nil {
		return 0, err
	}
	for page := 1; id == 0 && page <= 10; page++ {
		var items []remoteComment
		if err := client.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("%s/issues/%d/comments?per_page=100&page=%d", path, number, page), nil, &items); err != nil {
			return 0, err
		}
		for _, comment := range items {
			if comment.User.Type == "Bot" && comment.User.Login == client.credentials.Slug+"[bot]" && strings.HasPrefix(comment.Body, marker) {
				id = comment.ID
				break
			}
		}
		if id > 0 || len(items) < 100 {
			break
		}
		if page == 10 {
			return 0, fmt.Errorf("GitHub comment recovery exceeds supported bound")
		}
	}
	if id > 0 {
		return id, client.RepositoryRequest(ctx, token, "PATCH", fmt.Sprintf("%s/issues/comments/%d", path, id), map[string]string{"body": body}, nil)
	}
	var created remoteComment
	if err := client.RepositoryRequest(ctx, token, "POST", fmt.Sprintf("%s/issues/%d/comments", path, number), map[string]string{"body": body}, &created); err != nil {
		return 0, err
	}
	if created.ID <= 0 {
		return 0, fmt.Errorf("invalid GitHub comment identity")
	}
	return created.ID, nil
}
