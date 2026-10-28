//go:build integration

package cloud_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestObservationCannotResolveAnIdenticalNameInAnotherProject(t *testing.T) {
	ctx, workloads, store, principal := database(t)
	first := verifyConnection(t, ctx, store, principal)
	policy := store.Policies["sample"]
	policy.Scope.ProjectID = "other"
	store.Policies["other"] = policy
	if err := workloads.SeedProject(ctx, v1.Project{Scope: policy.Scope, Name: "Other cloud project", RepositoryID: 43, ClusterID: "keldock-m1"}); err != nil {
		t.Fatal(err)
	}
	request := cloud.ConnectionRequest{Provider: "aws", Account: "123456789012", Region: "eu-central-1", Profiles: []string{"private-storage"}, IdentityRef: "aws-default", Generation: 1}
	if _, err := store.Mutate(ctx, principal, "other", "other-project-connection", command(t, "connection", first.ID, request)); err != nil {
		t.Fatal(err)
	}
	report := cloud.Evidence{ProjectID: "other", ID: first.ID, Generation: 1, State: "ready", Reason: "OtherProjectEvidence", Details: map[string]string{"account": request.Account}, ObservedAt: time.Now().UTC()}
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", report); err != nil {
		t.Fatal(err)
	}
	original, err := store.List(ctx, principal, "sample")
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.List(ctx, principal, "other")
	if err != nil {
		t.Fatal(err)
	}
	if original[0].Observation.Reason != "SyntheticIdentityFixture" || observed[0].Observation.Reason != report.Reason {
		t.Fatal("observation crossed its project boundary")
	}
	report.ProjectID = ""
	if err := store.Observe(ctx, principal.User.OrganizationID, "keldock-m1", report); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("unscoped evidence accepted", err)
	}
}
