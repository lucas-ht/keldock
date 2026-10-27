package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

type recordingDriver struct{ mutations int }

func (driver *recordingDriver) Reconcile(context.Context, v1.ProjectedDeployment) error {
	driver.mutations++
	return nil
}

func (driver *recordingDriver) Observe(context.Context, v1.Placement, v1.Application, map[string]string, string) (v1.Observation, error) {
	return v1.Observation{}, nil
}

func (driver *recordingDriver) Remove(context.Context, v1.Placement) (bool, error) {
	driver.mutations++
	return true, nil
}

func (driver *recordingDriver) Suspend(context.Context, v1.Placement) error {
	driver.mutations++
	return nil
}

func TestAgentRevalidatesBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		changed bool
	}{
		{"selection superseded", http.StatusOK, true},
		{"credential revoked", http.StatusForbidden, false},
		{"control plane unavailable", http.StatusServiceUnavailable, false},
		{"selection removed", http.StatusNotFound, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			listed := v1.ProjectedDeployment{Operation: v1.Operation{ID: "op-original", EnvironmentID: "env-example", Generation: 1, Kind: "deploy", State: "reconciling"}, Bundle: "registry.example/bundle@sha256:" + strings.Repeat("a", 64)}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var response any
				if r.URL.Path == "/v1/agent/desired" {
					response = map[string]any{"apiVersion": v1.APIVersion, "items": []v1.ProjectedDeployment{listed}, "nextCursor": ""}
				} else if test.status != http.StatusOK {
					w.WriteHeader(test.status)
					return
				} else {
					current := listed
					if test.changed {
						current.Operation.Generation++
					}
					response = current
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, strings.Repeat("a", 32), true)
			if err != nil {
				t.Fatal(err)
			}
			driver := &recordingDriver{}
			runtime := Agent{Client: client, Driver: driver}
			err = runtime.Step(context.Background())
			if driver.mutations != 0 {
				t.Fatal("unconfirmed selection mutated the cluster")
			}
			if test.status == http.StatusForbidden {
				var status APIError
				if !errors.As(err, &status) || status.Status != test.status {
					t.Fatalf("revocation not surfaced: %v", err)
				}
				if err := runtime.Run(context.Background()); !errors.As(err, &status) {
					t.Fatal("revoked agent did not stop")
				}
			}
		})
	}
}

func TestClientDoesNotForwardCredentialsThroughRedirects(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client, err := NewClient(redirect.URL, strings.Repeat("a", 32), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Desired(context.Background()); err == nil || forwarded {
		t.Fatal("redirect forwarded the agent credential")
	}
	for _, endpoint := range []string{"http://control.example", "https://user:password@control.example", "https://control.example?token=hidden"} {
		if _, err := NewClient(endpoint, strings.Repeat("a", 32), true); err == nil {
			t.Fatal("insecure or ambiguous endpoint accepted")
		}
	}
}
