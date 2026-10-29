package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestSourceDoesNotAdvanceAcrossPartialComponentRegistration(t *testing.T) {
	ctx := context.Background()
	kinds := map[schema.GroupVersionResource]string{}
	for kind, resource := range resources {
		kinds[resource] = kind + "List"
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds)
	registrar := &Registrar{client: client, Repository: "registry.example/keldock/bundles", JournalNamespace: "keldock-system"}
	selection := compiler.Selection{EnvironmentID: "env-test", DeploymentID: "dep-first", Namespace: "ke-test", Generation: 1}
	desired := v1.ProjectedDeployment{
		Operation: v1.Operation{Scope: v1.Scope{OrganizationID: "team", ProjectID: "sample"}},
		Selection: selection,
		Definition: v1.Application{APIVersion: v1.APIVersion, Name: "sample", Components: map[string]v1.Component{
			"api": {Kind: "http", Size: "small", Port: 8080, Probe: &v1.Probe{Path: "/ready"}},
		}},
		Images: map[string]string{"api": "ghcr.io/example/app@sha256:" + strings.Repeat("a", 64)},
		Bundle: registrar.Repository + "@sha256:" + strings.Repeat("b", 64),
	}
	if err := registrar.Reconcile(ctx, desired); err != nil {
		t.Fatal(err)
	}
	failComponent := true
	client.PrependReactor("update", "kustomizations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if failComponent {
			return true, nil, errors.New("injected component registration failure")
		}
		return false, nil, nil
	})
	desired.Selection.Generation++
	desired.Selection.DeploymentID = "dep-second"
	desired.Bundle = registrar.Repository + "@sha256:" + strings.Repeat("c", 64)
	if err := registrar.Reconcile(ctx, desired); err == nil {
		t.Fatal("partial component registration unexpectedly succeeded")
	}
	source, err := registrar.resource("OCIRepository", selection.Namespace).Get(ctx, "keldock-source", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	digest, _, err := unstructured.NestedString(source.Object, "spec", "ref", "digest")
	if err != nil || digest != "sha256:"+strings.Repeat("b", 64) {
		t.Fatal("new artifact became visible before its component policy", err)
	}
	failComponent = false
	if err := registrar.Reconcile(ctx, desired); err != nil {
		t.Fatal(err)
	}
	source, err = registrar.resource("OCIRepository", selection.Namespace).Get(ctx, "keldock-source", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	digest, _, err = unstructured.NestedString(source.Object, "spec", "ref", "digest")
	if err != nil || digest != "sha256:"+strings.Repeat("c", 64) {
		t.Fatal("retry did not advance the source after successful registration", err)
	}
}
