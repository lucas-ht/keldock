//go:build integration

package github_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestDurableFeedbackRecoversRemoteEffectsWithoutTheRunner(t *testing.T) {
	f := integration(t)
	var check, deploy, comment map[string]any
	var checkCreates, deployCreates, commentCreates int
	var deploymentStatus []map[string]any
	lost := true
	f.remote.mu.Lock()
	f.remote.feedback = func(w http.ResponseWriter, r *http.Request, write func(any)) bool {
		read := func() map[string]any {
			var data map[string]any
			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				t.Error(err)
			}
			return data
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/check-runs") && r.Method == "GET":
			items := []map[string]any{}
			if check != nil {
				items = append(items, map[string]any{"id": 101, "external_id": check["external_id"], "app": map[string]any{"id": 501}})
			}
			write(map[string]any{"check_runs": items})
		case strings.HasSuffix(r.URL.Path, "/check-runs") && r.Method == "POST":
			check = read()
			checkCreates++
			if lost {
				lost = false
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
				} else {
					_ = connection.Close()
				}
				return true
			}
			write(map[string]int64{"id": 101})
		case strings.HasSuffix(r.URL.Path, "/check-runs/101"):
			for k, v := range read() {
				check[k] = v
			}
			write(map[string]int64{"id": 101})
		case strings.HasSuffix(r.URL.Path, "/deployments") && r.Method == "GET":
			items := []map[string]any{}
			if deploy != nil {
				items = append(items, map[string]any{"id": 201, "payload": deploy["payload"]})
			}
			write(items)
		case strings.HasSuffix(r.URL.Path, "/deployments") && r.Method == "POST":
			deploy = read()
			deployCreates++
			write(map[string]int64{"id": 201})
		case strings.HasSuffix(r.URL.Path, "/deployments/201/statuses"):
			if r.Method == "POST" {
				deploymentStatus = []map[string]any{read()}
				write(map[string]int64{"id": 401})
			} else {
				write(deploymentStatus)
			}
		case strings.HasSuffix(r.URL.Path, "/issues/1/comments"):
			if r.Method == "POST" {
				comment = read()
				commentCreates++
				write(map[string]int64{"id": 301})
			} else {
				items := []map[string]any{}
				if comment != nil {
					items = append(items, map[string]any{"id": 301, "body": comment["body"], "user": map[string]string{"login": "keldock-test[bot]", "type": "Bot"}})
				}
				write(items)
			}
		case strings.HasSuffix(r.URL.Path, "/issues/comments/301"):
			comment = read()
			write(map[string]int64{"id": 301})
		default:
			return false
		}
		return true
	}
	f.remote.mu.Unlock()
	data, err := os.ReadFile("../../examples/multi-service/keldock.yaml")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := v1.DecodeApplication(data)
	if err != nil {
		t.Fatal(err)
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: "sample", Definition: definition, Images: map[string]string{}, Provenance: json.RawMessage(`{}`), Source: v1.Source{RepositoryID: 100, OwnerID: 42, Repository: "owner/sample", Commit: strings.Repeat("b", 40), HeadCommit: strings.Repeat("a", 40), Ref: "refs/pull/1/merge", Event: "pull_request", WorkflowRef: "owner/sample/.github/workflows/deploy.yml@refs/pull/1/merge", RunID: 1, RunAttempt: 1, PullRequest: 1}}
	for name := range definition.Components {
		input.Images[name] = "ghcr.io/owner/image@sha256:" + strings.Repeat("c", 64)
	}
	receipt, err := f.service.Product.Admit(f.ctx, v1.Scope{OrganizationID: f.users.Organization, ProjectID: "sample"}, "github:test", "feedback-test", "preview.keldock.test", input)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Repository.DiscoverFeedback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.service.ProcessFeedback(f.ctx); !worked || err == nil {
		t.Fatal("lost remote create response was not retried durably")
	}
	process := func() {
		t.Helper()
		if _, err := f.service.Product.Pool().Exec(f.ctx, `UPDATE github_feedback SET available_at=now() WHERE organization_id=$1`, f.users.Organization); err != nil {
			t.Fatal(err)
		}
		if worked, err := f.service.ProcessFeedback(f.ctx); err != nil || !worked {
			t.Fatal(fmt.Sprintf("feedback unavailable: %v", err))
		}
	}
	process()
	if checkCreates != 1 || deployCreates != 1 || commentCreates != 1 {
		t.Fatal("lost response duplicated a remote effect")
	}
	if check["head_sha"] != input.Source.HeadCommit || deploy["ref"] != input.Source.Commit {
		t.Fatal("PR head and tested merge SHA were conflated")
	}
	if check["conclusion"] == "success" || deploymentStatus[0]["state"] == "success" {
		t.Fatal("acceptance was reported as readiness")
	}
	if _, err := f.service.Product.Pool().Exec(f.ctx, `UPDATE operations SET state='failed' WHERE id=$1`, receipt.OperationID); err != nil {
		t.Fatal(err)
	}
	process()
	if check["conclusion"] != "failure" || deploymentStatus[0]["state"] != "failure" || !strings.Contains(comment["body"].(string), "Deployment failed") || commentCreates != 1 {
		t.Fatal("durable outcome did not update the same feedback records")
	}
	process()
	if checkCreates != 1 || deployCreates != 1 || commentCreates != 1 {
		t.Fatal("unchanged replay duplicated feedback")
	}
}
