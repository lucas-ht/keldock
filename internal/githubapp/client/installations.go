package client

import (
	"context"
	"fmt"
	"regexp"
)

var repositoryPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,99}/[a-zA-Z0-9_.-]{1,100}$`)

// RepositoryPath accepts only one GitHub.com owner/name, never a URL or traversal.
func RepositoryPath(name string) (string, error) {
	if !repositoryPattern.MatchString(name) || name[len(name)-1] == '.' {
		return "", fmt.Errorf("invalid GitHub repository name")
	}
	return "/repos/" + name, nil
}

// App reads authenticated registration metadata and verifies the credential's App ID.
func (client *Client) App(ctx context.Context) (App, error) {
	var app App
	err := client.AppRequest(ctx, "GET", "/app", nil, &app)
	if err == nil && (app.ID != client.credentials.ID || app.Slug != client.credentials.Slug) {
		err = fmt.Errorf("GitHub App credentials do not match the configured identity")
	}
	return app, err
}

// Installations lists a bounded account delegation snapshot; it never invents selection.
func (client *Client) Installations(ctx context.Context) ([]Installation, error) {
	var result []Installation
	for page := 1; page <= 10; page++ {
		var items []Installation
		if err := client.AppRequest(ctx, "GET", fmt.Sprintf("/app/installations?per_page=100&page=%d", page), nil, &items); err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < 100 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("installation inventory exceeds the supported bound")
}

// Repositories obtains GitHub's actual selected repositories for one installation.
func (client *Client) Repositories(ctx context.Context, installation int64) ([]Repository, error) {
	token, err := client.InstallationToken(ctx, installation, nil)
	if err != nil {
		return nil, err
	}
	var result []Repository
	for page := 1; page <= 10; page++ {
		var response struct {
			Repositories []Repository `json:"repositories"`
		}
		if err := client.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page), nil, &response); err != nil {
			return nil, err
		}
		result = append(result, response.Repositories...)
		if len(response.Repositories) < 100 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("repository inventory exceeds the supported bound")
}

// PullRequests snapshots open PRs to recover events missed during an outage.
func (client *Client) PullRequests(ctx context.Context, token Token, repository string) ([]PullRequest, error) {
	path, err := RepositoryPath(repository)
	if err != nil {
		return nil, err
	}
	var result []PullRequest
	for page := 1; page <= 10; page++ {
		var items []PullRequest
		if err := client.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("%s/pulls?state=open&per_page=100&page=%d", path, page), nil, &items); err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < 100 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("pull request inventory exceeds the supported bound")
}
