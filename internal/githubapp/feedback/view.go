// Package feedback renders revision-bound GitHub feedback from durable product state.
package feedback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lucas-ht/keldock/internal/githubapp/store"
)

// View excludes observation timestamps so unchanged healthy polling produces no API writes.
type View struct {
	Title, Summary, Status, Conclusion, DeploymentState, URL, Head, Marker string
	Comment                                                                bool
}

// Render distinguishes acceptance, readiness, unavailable observation and supersession.
func Render(item store.Feedback, origin string, now time.Time) View {
	op, source := item.Operation, item.Release.Source
	view := View{Title: "Release accepted", Status: "queued", DeploymentState: "queued", Head: source.Commit,
		URL: fmt.Sprintf("%s/projects/%s/environments/%s", origin, op.ProjectID, op.EnvironmentID), Marker: fmt.Sprintf("keldock:%s:%s", op.OrganizationID, op.ID)}
	if source.PullRequest > 0 {
		view.Head = source.HeadCommit
	}
	view.Comment = source.PullRequest > 0 && item.LatestDeployment && (op.Generation == item.CurrentGeneration || item.CurrentKind == "delete") && (item.PullHead == source.HeadCommit || item.CurrentKind == "delete")
	switch op.State {
	case "publishing", "reconciling":
		view.Title, view.Status, view.DeploymentState = "Rollout in progress", "in_progress", "in_progress"
	case "ready":
		fresh := item.Report != nil && item.Report.Generation == op.Generation && item.Report.DeploymentID == op.DeploymentID && item.Report.Bundle == item.Bundle && item.Report.State == "ready" && now.Sub(item.Report.ObservedAt) <= 30*time.Second
		if fresh {
			view.Title, view.Status, view.Conclusion, view.DeploymentState = "Deployment ready", "completed", "success", "success"
		} else {
			view.Title, view.Status, view.Conclusion, view.DeploymentState = "Readiness observation unavailable", "completed", "action_required", "error"
		}
	case "failed":
		view.Title, view.Status, view.Conclusion, view.DeploymentState = "Deployment failed", "completed", "failure", "failure"
	case "canceled":
		view.Title, view.Status, view.Conclusion, view.DeploymentState = "Operation canceled; completed effects retained", "completed", "cancelled", "inactive"
	case "superseded":
		view.Title, view.Status, view.Conclusion, view.DeploymentState = "Release superseded", "completed", "neutral", "inactive"
	}
	if item.LatestDeployment && item.CurrentKind == "delete" {
		view.Title = "Preview removal in progress"
		if item.CurrentState == "deleted" {
			view.Title = "Preview removed"
		}
		if item.CurrentState == "failed" {
			view.Title = "Preview removal failed"
		}
	}
	view.Summary = fmt.Sprintf("%s.\n\nPrebuilt release accepted by Keldock. Desired generation: **%d**.\n\nTested commit: `%s`.\n", view.Title, op.Generation, source.Commit)
	if source.PullRequest > 0 {
		view.Summary += fmt.Sprintf("\nPR head: `%s`. The tested commit may be GitHub's merge commit.\n", source.HeadCommit)
	}
	view.Summary += fmt.Sprintf("\n[Deployment details](%s) · Operation `%s`.\n\nThe Actions wait deadline does not cancel this operation.", view.URL, op.ID)
	return view
}

// Hash identifies the exact feedback projection successfully delivered remotely.
func (view View) Hash() string {
	data, _ := json.Marshal(view)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// CommentBody uses one stable installation/repository marker, not one comment per run.
func (view View) CommentBody(organization string, repo, number int64) string {
	return fmt.Sprintf("<!-- keldock:%s:%d:%d -->\n### Keldock deployment\n\n%s", organization, repo, number, view.Summary)
}
