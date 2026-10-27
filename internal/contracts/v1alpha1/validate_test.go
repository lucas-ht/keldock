package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

func TestObservationRequiresActualSelectedWorkloads(t *testing.T) {
	definition := validApplication()
	newReport := func() Observation {
		report := Observation{EnvironmentID: "env-one", DeploymentID: "dep-one", Generation: 1, Bundle: "registry.example/bundles@sha256:" + strings.Repeat("c", 64), State: "ready", ObservedAt: time.Now().UTC()}
		for name := range definition.Components {
			report.Components = append(report.Components, ComponentHealth{Name: name, Ready: true, ObjectUID: "controller-" + name, ResourceVersion: "1", WorkloadUID: "workload-" + name, WorkloadVersion: "2", Image: "registry.example/app@sha256:" + strings.Repeat("a", 64), ObservedGeneration: 1, AppliedRevision: "sha256:" + strings.Repeat("c", 64)})
		}
		return report
	}
	if err := newReport().Validate(definition); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*Observation)
	}{
		{"empty successful build", func(r *Observation) { r.Components = nil }},
		{"missing workload", func(r *Observation) { r.Components[0].WorkloadUID = "" }},
		{"missing resource version", func(r *Observation) { r.Components[0].WorkloadVersion = "" }},
		{"old source", func(r *Observation) { r.Components[0].AppliedRevision = "sha256:" + strings.Repeat("b", 64) }},
		{"mutable image", func(r *Observation) { r.Components[0].Image = "registry.example/app:latest" }},
		{"duplicate component", func(r *Observation) { r.Components[1] = r.Components[0] }},
		{"unknown component", func(r *Observation) { r.Components[0].Name = "unrequested" }},
		{"unbounded output", func(r *Observation) { r.Components[0].Message = strings.Repeat("x", 1025) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := newReport()
			test.change(&report)
			if err := report.Validate(definition); err == nil {
				t.Fatal("invalid readiness evidence accepted")
			}
		})
	}
	scope := Scope{OrganizationID: "org-one", ProjectID: "project"}
	first := EnvironmentID(scope, "sample", 7)
	if first != EnvironmentID(scope, "sample", 7) || !ValidName(first) || first == EnvironmentID(scope, "sample", 8) || first == EnvironmentID(Scope{OrganizationID: "org-two", ProjectID: "project"}, "sample", 7) {
		t.Fatal("environment identity lost stability or ownership scope")
	}
}

func FuzzStrictJSON(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"components":{"a":{"kind":"http"}}}`, `{"name":"first","name":"second"}`, `[1,2,3]`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 1024*1024 {
			t.Skip()
		}
		var app Application
		if err := DecodeJSON([]byte(data), &app); err == nil {
			_ = app.Validate()
		} // Validation errors are expected for arbitrary parser inputs.
	})
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
