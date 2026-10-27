package feedback_test

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/githubapp/feedback"
	"github.com/lucas-ht/keldock/internal/githubapp/store"
)

func TestReadinessAndCommentStayBoundToCurrentEvidence(t *testing.T) {
	now := time.Now()
	item := store.Feedback{Operation: v1.Operation{Scope: v1.Scope{OrganizationID: "org", ProjectID: "sample"}, ID: "op-test", EnvironmentID: "env-test", DeploymentID: "dep-test", Generation: 2, State: "ready"}, CurrentGeneration: 2, LatestDeployment: true, PullHead: strings.Repeat("a", 40), Bundle: "bundle"}
	item.Release.Source = v1.Source{Commit: strings.Repeat("b", 40), HeadCommit: item.PullHead, PullRequest: 1}
	view := feedback.Render(item, "https://keldock.example", now)
	if view.Conclusion == "success" || !view.Comment {
		t.Fatal("missing observation was reported ready")
	}
	item.Report = &v1.Observation{Generation: 2, DeploymentID: "dep-test", Bundle: "bundle", State: "ready", ObservedAt: now}
	view = feedback.Render(item, "https://keldock.example", now)
	if view.Conclusion != "success" || view.Head != item.PullHead || !strings.Contains(view.Summary, item.Release.Source.Commit) {
		t.Fatal("fresh evidence or tested identity lost")
	}
	if stale := feedback.Render(item, "https://keldock.example", now.Add(time.Minute)); stale.Conclusion == "success" {
		t.Fatal("stale observation remained successful")
	}
	item.CurrentGeneration = 3
	item.PullHead = strings.Repeat("c", 40)
	if older := feedback.Render(item, "https://keldock.example", now); older.Comment {
		t.Fatal("old build could overwrite current PR comment")
	}
}
