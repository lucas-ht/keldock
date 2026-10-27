package githubapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/githubapp/client"
	"github.com/lucas-ht/keldock/internal/githubapp/store"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func permissionsComplete(permissions map[string]string) bool {
	for name, required := range RequiredPermissions() {
		actual := permissions[name]
		if actual != required && actual != "write" {
			return false
		}
	}
	return true
}

// Sync verifies remote selection before changing local grants, then recovers missed PR state.
func (service *Service) Sync(ctx context.Context) (result error) {
	defer func() {
		if result != nil {
			if err := service.Repository.SyncFailure(ctx); err != nil {
				result = fmt.Errorf("GitHub sync and failure recording unavailable: %w", result)
			}
		}
	}()
	api, app, err := service.active(ctx)
	if err != nil {
		return err
	}
	metadata, err := api.App(ctx)
	if err != nil {
		return err
	}
	installations, err := api.Installations(ctx)
	if err != nil {
		return err
	}
	owners := map[int64]bool{}
	for _, owner := range service.Config.RepositoryOwners {
		owners[owner] = true
	}
	snapshots := []store.Snapshot{}
	for _, installation := range installations {
		if installation.AppID != app.ID || !owners[installation.Account.ID] {
			continue
		}
		snapshot := store.Snapshot{Installation: installation}
		if installation.SuspendedAt == nil && permissionsComplete(installation.Permissions) && permissionsComplete(metadata.Permissions) {
			repositories, err := api.Repositories(ctx, installation.ID)
			if err != nil {
				return err
			}
			for _, repository := range repositories {
				if _, err := client.RepositoryPath(repository.FullName); err != nil {
					return err
				}
				project := ""
				for id, configured := range service.Config.Projects {
					if configured.RepositoryID == repository.ID && service.Config.RepositoryOwners[repository.ID] == repository.Owner.ID {
						project = id
					}
				}
				snapshot.Repositories = append(snapshot.Repositories, store.RepositoryGrant{ID: repository.ID, InstallationID: installation.ID, OwnerID: repository.Owner.ID, FullName: repository.FullName, ProjectID: project})
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := service.Repository.ReplaceSnapshot(ctx, metadata, fingerprint(app), snapshots); err != nil {
		return err
	}
	if !permissionsComplete(metadata.Permissions) {
		return fmt.Errorf("GitHub App is missing required repository permissions")
	}
	_, grants, err := service.Repository.Inventory(ctx)
	if err != nil {
		return err
	}
	for _, grant := range grants {
		if grant.ProjectID == "" || !grant.Active {
			continue
		}
		if err := service.recoverPullRequests(ctx, api, grant); err != nil {
			return err
		}
	}
	return service.Repository.PruneEvents(ctx)
}

func (service *Service) recoverPullRequests(ctx context.Context, api *client.Client, grant store.RepositoryGrant) error {
	token, err := api.InstallationToken(ctx, grant.InstallationID, []int64{grant.ID})
	if err != nil {
		return err
	}
	requests, err := api.PullRequests(ctx, token, grant.FullName)
	if err != nil {
		return err
	}
	open := map[int64]bool{}
	for _, request := range requests {
		open[request.Number] = true
		if err := service.recordPull(ctx, grant, request); err != nil {
			return err
		}
	}
	tracked, err := service.Repository.TrackedPulls(ctx, grant.ProjectID)
	if err != nil {
		return err
	}
	path, err := client.RepositoryPath(grant.FullName)
	if err != nil {
		return err
	}
	for _, number := range tracked {
		if open[number] {
			continue
		}
		var request client.PullRequest
		if err := api.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("%s/pulls/%d", path, number), nil, &request); err != nil {
			return err
		}
		if err := service.recordPull(ctx, grant, request); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) recordPull(ctx context.Context, grant store.RepositoryGrant, request client.PullRequest) error {
	if request.Number <= 0 || (request.State != "open" && request.State != "closed") {
		return access.ErrInvalid
	}
	if request.Head.Repo.ID == 0 {
		return nil
	} // A deleted fork cannot acquire deployment authority.
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(append([]byte(service.Repository.Organization+"/"+grant.ProjectID+"/"), data...))
	hash := hex.EncodeToString(sum[:])
	_, err = service.Product.RecordPullRequest(ctx, v1.Scope{OrganizationID: service.Repository.Organization, ProjectID: grant.ProjectID}, "github-sync-"+hash, hash, postgres.PullRequestUpdate{RepositoryID: grant.ID, Number: request.Number, HeadCommit: request.Head.SHA, HeadRepositoryID: request.Head.Repo.ID, Closed: request.State == "closed", UpdatedAt: request.UpdatedAt})
	return err
}

// AuthorizeRepository verifies local policy and current remote access before using CI authority.
func (service *Service) AuthorizeRepository(ctx context.Context, repositoryID, ownerID int64, name string) (store.RepositoryGrant, error) {
	if _, err := client.RepositoryPath(name); err != nil {
		return store.RepositoryGrant{}, access.ErrDenied
	}
	grant, err := service.Repository.Grant(ctx, repositoryID)
	if err != nil {
		return grant, err
	}
	if grant.ProjectID == "" || grant.OwnerID != ownerID || service.Config.RepositoryOwners[repositoryID] != ownerID {
		return grant, access.ErrDenied
	}
	api, _, err := service.active(ctx)
	if err != nil {
		return grant, err
	}
	token, err := api.InstallationToken(ctx, grant.InstallationID, []int64{repositoryID})
	if err != nil {
		return grant, delegationFailure(err)
	}
	var remote client.Repository
	if err := api.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("/repositories/%d", repositoryID), nil, &remote); err != nil {
		return grant, delegationFailure(err)
	}
	// A run or receipt may preserve the old name after a rename. Verified stable
	// repository/owner IDs grant authority; GitHub's current name supplies routing.
	if remote.ID != repositoryID || remote.Owner.ID != ownerID {
		return grant, access.ErrDenied
	}
	grant.FullName = remote.FullName
	return grant, nil
}

func delegationFailure(err error) error {
	var upstream *client.Error
	if errors.As(err, &upstream) && !upstream.Limited && (upstream.Status == 401 || upstream.Status == 403 || upstream.Status == 404 || upstream.Status == 422) {
		return access.ErrDenied
	}
	return err
}

// AuthorizeRelease refreshes authoritative PR state before transactional receipt/admission checks.
func (service *Service) AuthorizeRelease(ctx context.Context, source v1.Source) error {
	grant, err := service.AuthorizeRepository(ctx, source.RepositoryID, source.OwnerID, source.Repository)
	if err != nil {
		return err
	}
	if source.PullRequest == 0 {
		return nil
	}
	api, _, err := service.active(ctx)
	if err != nil {
		return err
	}
	token, err := api.InstallationToken(ctx, grant.InstallationID, []int64{grant.ID})
	if err != nil {
		return err
	}
	path, err := client.RepositoryPath(grant.FullName)
	if err != nil {
		return err
	}
	var request client.PullRequest
	if err := api.RepositoryRequest(ctx, token, "GET", fmt.Sprintf("%s/pulls/%d", path, source.PullRequest), nil, &request); err != nil {
		return err
	}
	if request.Number != source.PullRequest {
		return access.ErrDenied
	}
	return service.recordPull(ctx, grant, request)
}
