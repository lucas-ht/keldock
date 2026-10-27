package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"
)

func validApplication() Application {
	return Application{APIVersion: APIVersion, Name: "sample", Components: map[string]Component{
		"web":      {Kind: "http", Size: "small", Port: 8080, Probe: &Probe{Path: "/ready"}, DependsOn: []string{"migrate"}, ConnectsTo: []string{"api"}},
		"api":      {Kind: "http", Size: "small", Port: 8080, Probe: &Probe{Path: "/ready"}, ConnectsTo: []string{"web"}},
		"migrate":  {Kind: "job", Size: "small", Command: []string{"/app/migrate"}},
		"worker":   {Kind: "worker", Size: "small", Probe: &Probe{Command: []string{"/app/health"}}},
		"schedule": {Kind: "cronjob", Size: "small", Schedule: "*/5 * * * *"},
	}}
}

func TestApplicationContract(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Application)
		valid  bool
	}{
		{"complete graph with cyclic communication", func(*Application) {}, true},
		{"version", func(a *Application) { a.APIVersion = "other/v1" }, false},
		{"invalid identity", func(a *Application) { a.Name = "../escape" }, false},
		{"empty", func(a *Application) { a.Components = nil }, false},
		{"unknown kind", func(a *Application) { a.Components["other"] = Component{Kind: "raw-pod", Size: "small"} }, false},
		{"unsupported profile", func(a *Application) { c := a.Components["web"]; c.Size = "privileged"; a.Components["web"] = c }, false},
		{"missing probe", func(a *Application) { c := a.Components["web"]; c.Probe = nil; a.Components["web"] = c }, false},
		{"ambiguous probe", func(a *Application) { a.Components["web"].Probe.Command = []string{"true"} }, false},
		{"missing dependency", func(a *Application) {
			c := a.Components["web"]
			c.DependsOn = []string{"missing"}
			a.Components["web"] = c
		}, false},
		{"duplicate dependency", func(a *Application) {
			c := a.Components["web"]
			c.DependsOn = []string{"api", "api"}
			a.Components["web"] = c
		}, false},
		{"rollout cycle", func(a *Application) {
			web := a.Components["web"]
			web.DependsOn = []string{"api"}
			a.Components["web"] = web
			api := a.Components["api"]
			api.DependsOn = []string{"web"}
			a.Components["api"] = api
		}, false},
		{"invalid schedule", func(a *Application) {
			c := a.Components["schedule"]
			c.Schedule = "every day"
			a.Components["schedule"] = c
		}, false},
		{"job route", func(a *Application) { c := a.Components["migrate"]; c.Route = true; a.Components["migrate"] = c }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := validApplication()
			test.change(&app)
			if err := app.Validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
	order, err := validApplication().Order()
	if err != nil || strings.Index(strings.Join(order, ","), "migrate") > strings.Index(strings.Join(order, ","), "web") {
		t.Fatalf("prerequisite order: %v, %v", order, err)
	}
}

func TestPrebuiltReleaseContract(t *testing.T) {
	request := SubmitRelease{APIVersion: APIVersion, ProjectID: "project", Definition: validApplication(),
		Source: Source{RepositoryID: 1, OwnerID: 2, Repository: "sample/app", Commit: strings.Repeat("a", 40),
			Ref: "refs/pull/7/merge", Event: "pull_request", WorkflowRef: "sample/app/.github/workflows/build.yaml@refs/pull/7/merge", RunID: 3, RunAttempt: 1, PullRequest: 7},
		Images: map[string]string{}, Provenance: json.RawMessage(`{"payload":"signed separately"}`)}
	for component := range request.Definition.Components {
		request.Images[component] = "registry.example/sample@sha256:" + strings.Repeat("b", 64)
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Images["web"] = "registry.example/sample:latest"
	if err := request.Validate(); err == nil {
		t.Fatal("mutable image was admitted")
	}
	delete(request.Images, "web")
	if err := request.Validate(); err == nil {
		t.Fatal("partial release was admitted")
	}
}

func TestStrictDecoding(t *testing.T) {
	for _, data := range []string{
		`{"apiVersion":"x","apiVersion":"y"}`,
		`{"apiVersion":"x","privileged":true}`,
		`{"apiVersion":"x"} {}`,
		`{"components":{"a":{"kind":"worker","kind":"http"}}}`,
	} {
		var app Application
		if err := DecodeJSON([]byte(data), &app); err == nil {
			t.Errorf("accepted invalid JSON %s", data)
		}
	}
	for _, data := range []string{
		"apiVersion: x\napiVersion: y\n",
		"apiVersion: x\nprivileged: true\n",
		"apiVersion: x\n---\napiVersion: y\n",
	} {
		if _, err := DecodeApplication([]byte(data)); err == nil {
			t.Errorf("accepted invalid YAML %s", data)
		}
	}
}
