//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/cluster"
)

func TestFluxOCIContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	input := applicationInput(t)
	selection := selectedEnvironment()
	driver := registrar(t)
	dependencies, err := driver.Inspect(ctx)
	if err != nil || len(dependencies) != 3 {
		t.Fatalf("supplied dependency detection: %+v %v", dependencies, err)
	}
	for _, dependency := range dependencies {
		if dependency.Mode != "supplied" || dependency.UID == "" {
			t.Fatal("dependency discovery did not preserve supplied ownership")
		}
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		cleanEnvironment(t, cleanup, driver, selection)
	})
	bundle := publishBundle(t, ctx, input, selection)
	if err := driver.Reconcile(ctx, projected(selection, input, bundle)); err != nil {
		t.Fatal(err)
	}
	report := waitState(t, ctx, driver, selection, input, bundle, "ready")
	t.Logf("ready environment %s, route %s", selection.Namespace, selection.Hostname)
	if len(report.Components) != 5 {
		t.Fatal("application components missing")
	}
	for _, component := range report.Components {
		if component.Name == "schedule" && component.Reason != "ScheduleConfigured" {
			t.Fatal("scheduled execution was conflated with configuration readiness")
		}
	}
	response := route(t, ctx, selection)
	api, ok := response["api"].(map[string]any)
	if !ok || api["migrations"] != float64(1) || api["workerTicks"].(float64) < 1 {
		t.Fatalf("application graph did not run: %+v", response)
	}
	for _, args := range [][]string{
		{"patch", "namespaces"},
		{"patch", "roles.rbac.authorization.k8s.io", "-n", selection.Namespace},
		{"patch", "kustomizations.kustomize.toolkit.fluxcd.io", "-n", selection.Namespace},
		{"get", "secrets", "-n", selection.Namespace},
		{"create", "deployments.apps", "-n", "keldock-system"},
	} {
		command := append([]string{"auth", "can-i"}, args...)
		command = append(command, "--as=system:serviceaccount:"+selection.Namespace+":reconciler")
		data, err := kube(t, ctx, command...)
		verdict := strings.TrimSpace(string(data))
		verdict = verdict[strings.LastIndex(verdict, "\n")+1:]
		if err == nil || verdict != "no" {
			t.Fatalf("scoped identity unexpectedly allowed %v: %s %v", args, data, err)
		}
	}
	if data, err := kube(t, ctx, "exec", "-n", selection.Namespace, "deployment/web", "--", "/fixture", "probe", "http://api:8080/ready"); err != nil {
		t.Fatalf("allowed service connection failed: %s %v", data, err)
	}
	previous := selection
	selection.Generation++
	selection.DeploymentID = "dep-" + strings.TrimPrefix(selectedEnvironment().DeploymentID, "dep-")
	apiComponent := input.Definition.Components["api"]
	apiComponent.Env["BROKEN"] = "true"
	input.Definition.Components["api"] = apiComponent
	bundle = publishBundle(t, ctx, input, selection)
	if err := driver.Reconcile(ctx, projected(selection, input, bundle)); err != nil {
		t.Fatal(err)
	}
	immediate, err := driver.Observe(ctx, selection, input.Definition, input.Images, bundle)
	if err != nil || immediate.State == "ready" {
		t.Fatalf("old healthy revision satisfied new selection: %+v %v", immediate, err)
	}
	if err := driver.Reconcile(ctx, projected(previous, input, bundle)); !errors.Is(err, cluster.ErrOwnership) {
		t.Fatalf("older generation replaced current registration: %v", err)
	}
	waitState(t, ctx, driver, selection, input, bundle, "failed")
	delete(input.Definition.Components["api"].Env, "BROKEN")
	selection.Generation++
	selection.DeploymentID = selectedEnvironment().DeploymentID
	bundle = publishBundle(t, ctx, input, selection)
	if err := driver.Reconcile(ctx, projected(selection, input, bundle)); err != nil {
		t.Fatal(err)
	}
	waitState(t, ctx, driver, selection, input, bundle, "ready")
	delete(input.Definition.Components, "schedule")
	delete(input.Images, "schedule")
	selection.Generation++
	selection.DeploymentID = selectedEnvironment().DeploymentID
	bundle = publishBundle(t, ctx, input, selection)
	if err := driver.Reconcile(ctx, projected(selection, input, bundle)); err != nil {
		t.Fatal(err)
	}
	waitState(t, ctx, driver, selection, input, bundle, "ready")
	if data, err := kube(t, ctx, "get", "cronjob", "schedule", "-n", selection.Namespace, "-o", "json"); err == nil || !strings.Contains(string(data), "NotFound") {
		t.Fatalf("retired component was not pruned: %s %v", data, err)
	}
	input = applicationInput(t)
	selection.Generation++
	selection.DeploymentID = selectedEnvironment().DeploymentID
	bundle = publishBundle(t, ctx, input, selection)
	if err := driver.Reconcile(ctx, projected(selection, input, bundle)); err != nil {
		t.Fatal(err)
	}
	waitState(t, ctx, driver, selection, input, bundle, "ready")
	if data, err := kube(t, ctx, "get", "cronjob", "schedule", "-n", selection.Namespace, "-o", "name"); err != nil {
		t.Fatalf("rollback did not restore retired component: %s %v", data, err)
	}
	foreign := selection
	foreign.Namespace = "keldock-system"
	if err := driver.Reconcile(ctx, projected(foreign, input, bundle)); !errors.Is(err, cluster.ErrOwnership) {
		t.Fatalf("unowned namespace was adopted: %v", err)
	}
	other := selectedEnvironment()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		cleanEnvironment(t, cleanup, driver, other)
	})
	otherBundle := publishBundle(t, ctx, input, other)
	if err := driver.Reconcile(ctx, projected(other, input, otherBundle)); err != nil {
		t.Fatal(err)
	}
	waitState(t, ctx, driver, other, input, otherBundle, "ready")
	if data, err := kube(t, ctx, "exec", "-n", selection.Namespace, "deployment/web", "--", "/fixture", "probe", "http://api."+other.Namespace+".svc.cluster.local:8080/ready"); err == nil {
		t.Fatalf("cross-preview traffic succeeded: %s", data)
	}
	if data, err := kube(t, ctx, "exec", "-n", other.Namespace, "deployment/web", "--", "/fixture", "probe", "http://api:8080/ready"); err != nil {
		t.Fatalf("second preview's permitted traffic failed: %s %v", data, err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("verified component readiness evidence: %s", data)
}
