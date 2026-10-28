package cluster

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestJournalFencesDeletedNamespaceResurrection(t *testing.T) {
	r := &Registrar{client: fake.NewSimpleDynamicClient(runtime.NewScheme()), JournalNamespace: "keldock-system"}
	ctx := context.Background()
	first := JournalEntry{Scope: v1.Scope{OrganizationID: "org", ProjectID: "sample"}, EnvironmentID: "env-example", Generation: 1, Action: "deploy", DeploymentID: "dep-one", Bundle: "registry.example/bundle@sha256:aaaa"}
	if err := r.checkpoint(ctx, first); err != nil {
		t.Fatal(err)
	}
	deleted := JournalEntry{EnvironmentID: first.EnvironmentID, Generation: 2, Action: "delete"}
	if err := r.checkpoint(ctx, deleted); err != nil {
		t.Fatal(err)
	}
	if err := r.checkpoint(ctx, first); !errors.Is(err, ErrOwnership) {
		t.Fatalf("old snapshot resurrected deleted environment: %v", err)
	}
	first.Generation = 2
	if err := r.checkpoint(ctx, first); !errors.Is(err, ErrOwnership) {
		t.Fatalf("conflicting same generation replaced tombstone: %v", err)
	}
	first.Generation = 3
	if err := r.checkpoint(ctx, first); err != nil {
		t.Fatal(err)
	}
	paused := first
	paused.Action = "suspend"
	paused.Bundle = ""
	if err := r.checkpoint(ctx, paused); err != nil {
		t.Fatal(err)
	}
	if err := r.checkpoint(ctx, first); !errors.Is(err, ErrOwnership) {
		t.Fatal("canceled revision resumed without new generation")
	}
}
