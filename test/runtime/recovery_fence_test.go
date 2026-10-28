//go:build integration

package runtime_test

import (
	"testing"

	"github.com/lucas-ht/keldock/internal/cluster"
)

func TestRestoredControlPlaneFencesSurvivingNewerSelections(t *testing.T) {
	s, ctx := database(t)
	scope, input := scopedInput(t, s, ctx)
	r, err := s.Admit(ctx, scope, "ci", "recovery-fixture", "example.localhost", input)
	if err != nil {
		t.Fatal(err)
	}
	entry := cluster.JournalEntry{Scope: scope, EnvironmentID: r.EnvironmentID, Generation: r.Generation + 1, Action: "delete"}
	if err := s.FenceRecovery(ctx, scope.OrganizationID, "snapshot-fixture"); err != nil {
		t.Fatal(err)
	}
	if fenced, err := s.RecoveryFenced(ctx, scope.OrganizationID); err != nil || !fenced {
		t.Fatalf("restored state was not fenced: %v", err)
	}
	if err := s.ResumeRecovery(ctx, scope.OrganizationID, []cluster.JournalEntry{entry}); err == nil {
		t.Fatal("older snapshot resumed against removed/newer environment")
	}
	if fenced, err := s.RecoveryFenced(ctx, scope.OrganizationID); err != nil || !fenced {
		t.Fatal("failed review removed recovery fence")
	}
	entry.Action = "deploy"
	entry.Generation = r.Generation
	entry.DeploymentID = r.DeploymentID
	if err := s.ResumeRecovery(ctx, scope.OrganizationID, []cluster.JournalEntry{entry}); err != nil {
		t.Fatal(err)
	}
	if fenced, err := s.RecoveryFenced(ctx, scope.OrganizationID); err != nil || fenced {
		t.Fatal("verified recovery stayed fenced")
	}
}
