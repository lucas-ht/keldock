package githubapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lucas-ht/keldock/internal/githubapp/client"
	"github.com/lucas-ht/keldock/internal/githubapp/feedback"
	"github.com/lucas-ht/keldock/internal/githubapp/store"
)

// ProcessFeedback projects one durable receipt even when the submitting runner is gone.
func (service *Service) ProcessFeedback(ctx context.Context) (bool, error) {
	item, err := service.Repository.ClaimFeedback(ctx)
	if err != nil || item == nil {
		return false, err
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	view := feedback.Render(*item, service.Config.PublicURL, time.Now())
	if item.SentHash != view.Hash() {
		err = service.deliverFeedback(work, item, view)
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	retry := 15 * time.Second
	if err != nil {
		retry = 5 * time.Second * time.Duration(1<<min(item.Attempt, 9))
		var upstream *client.Error
		if errors.As(err, &upstream) {
			retry = max(retry, upstream.RetryAfter)
		}
	}
	if finish := service.Repository.FinishFeedback(ctx, *item, view.Hash(), retry, err == nil); finish != nil {
		return true, finish
	}
	return true, err
}

func (service *Service) deliverFeedback(ctx context.Context, item *store.Feedback, view feedback.View) error {
	source := item.Release.Source
	grant, err := service.AuthorizeRepository(ctx, source.RepositoryID, source.OwnerID, source.Repository)
	if err != nil {
		// Rename changes routing, never the stable repository/owner authority.
		current, lookup := service.Repository.Grant(ctx, source.RepositoryID)
		if lookup != nil {
			return err
		}
		grant, err = service.AuthorizeRepository(ctx, source.RepositoryID, source.OwnerID, current.FullName)
		if err != nil {
			return err
		}
	}
	api, _, err := service.active(ctx)
	if err != nil {
		return err
	}
	token, err := api.InstallationToken(ctx, grant.InstallationID, []int64{grant.ID})
	if err != nil {
		return err
	}
	check := map[string]any{"status": view.Status, "details_url": view.URL, "output": map[string]string{"title": view.Title, "summary": view.Summary}}
	if view.Conclusion != "" {
		check["conclusion"] = view.Conclusion
	}
	item.CheckID, err = api.Check(ctx, token, grant.FullName, view.Head, view.Marker, item.CheckID, check)
	if err != nil {
		return err
	}
	if err := service.Repository.SaveFeedbackIDs(ctx, *item); err != nil {
		return err
	}
	if item.DeploymentID == 0 {
		item.DeploymentID, err = api.Deployment(ctx, token, grant.FullName, source.Commit, item.Operation.EnvironmentID, view.Marker, source.PullRequest > 0)
		if err != nil {
			return err
		}
		if err := service.Repository.SaveFeedbackIDs(ctx, *item); err != nil {
			return err
		}
	}
	if err := api.DeploymentStatus(ctx, token, grant.FullName, item.DeploymentID, view.DeploymentState, view.Title, view.URL, view.Marker); err != nil {
		return err
	}
	if !view.Comment {
		return nil
	}
	id, err := service.Repository.ClaimComment(ctx, grant.ID, source.PullRequest, item.Lease)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = service.Repository.FinishComment(cleanup, grant.ID, source.PullRequest, id, item.Lease)
	}()
	current, err := service.Repository.CommentCurrent(ctx, *item)
	if err != nil || !current {
		return err
	}
	marker := fmt.Sprintf("<!-- keldock:%s:%d:%d -->", service.Repository.Organization, grant.ID, source.PullRequest)
	id, err = api.Comment(ctx, token, grant.FullName, source.PullRequest, id, marker, view.CommentBody(service.Repository.Organization, grant.ID, source.PullRequest))
	return err
}
