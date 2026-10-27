package v1alpha1

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/robfig/cron/v3"
)

var (
	namePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	digestPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$`)
	shaPattern    = regexp.MustCompile(`^[a-f0-9]{40}$`)
	envPattern    = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// ValidName reports whether a value can be used as a bounded product identifier.
func ValidName(name string) bool { return namePattern.MatchString(name) }

// Validate rejects unsupported or privilege-bearing application input.
func (app Application) Validate() error {
	if app.APIVersion != APIVersion || !ValidName(app.Name) {
		return fmt.Errorf("application requires apiVersion %s and a valid name", APIVersion)
	}
	if len(app.Components) < 1 || len(app.Components) > 16 {
		return fmt.Errorf("application must contain 1–16 components")
	}
	routes := 0
	for name, component := range app.Components {
		if !ValidName(name) || strings.HasPrefix(name, "keldock-") {
			return fmt.Errorf("invalid component name %q", name)
		}
		if err := component.validate(); err != nil {
			return fmt.Errorf("component %s: %w", name, err)
		}
		if component.Route {
			routes++
		}
		for _, list := range [][]string{component.DependsOn, component.ConnectsTo} {
			seen := map[string]bool{}
			for _, dependency := range list {
				if _, exists := app.Components[dependency]; !exists || dependency == name || seen[dependency] {
					return fmt.Errorf("component %s: invalid or duplicate dependency %q", name, dependency)
				}
				seen[dependency] = true
			}
		}
		for _, target := range component.ConnectsTo {
			if app.Components[target].Port == 0 {
				return fmt.Errorf("component %s connects to %s without a port", name, target)
			}
		}
	}
	if routes > 1 {
		return fmt.Errorf("first runtime supports one routed component per application")
	}
	_, err := app.Order()
	return err
}

func (component Component) validate() error {
	if !slices.Contains([]string{"http", "worker", "job", "cronjob"}, component.Kind) {
		return fmt.Errorf("unsupported kind %q", component.Kind)
	}
	if component.Size != "small" {
		return fmt.Errorf("unsupported size profile %q", component.Size)
	}
	if len(component.Command) > 32 || len(component.Args) > 64 || len(component.Env) > 32 {
		return fmt.Errorf("command, arguments or environment exceed supported bounds")
	}
	for key, value := range component.Env {
		if !envPattern.MatchString(key) || len(value) > 4096 {
			return fmt.Errorf("invalid or oversized environment entry %q", key)
		}
	}
	if component.Kind == "http" && (component.Port < 1024 || component.Port > 65535) {
		return fmt.Errorf("http requires a port between 1024 and 65535")
	}
	if component.Kind != "http" && (component.Port != 0 || component.Route) {
		return fmt.Errorf("only http components can expose a port or route")
	}
	if component.Kind == "cronjob" {
		fields := strings.Fields(component.Schedule)
		if len(fields) != 5 {
			return fmt.Errorf("cronjob requires a five-field schedule")
		}
		if _, err := cron.ParseStandard(component.Schedule); err != nil {
			return fmt.Errorf("invalid schedule: %w", err)
		}
	} else if component.Schedule != "" {
		return fmt.Errorf("schedule is only valid for cronjob")
	}
	if component.Kind == "http" || component.Kind == "worker" {
		if component.Probe == nil {
			return fmt.Errorf("long-running components require a probe")
		}
	} else if component.Probe != nil {
		return fmt.Errorf("jobs cannot define readiness probes")
	}
	if probe := component.Probe; probe != nil {
		if (probe.Path == "") == (len(probe.Command) == 0) {
			return fmt.Errorf("probe requires exactly one path or command")
		}
		if probe.Path != "" && (component.Port == 0 || !strings.HasPrefix(probe.Path, "/") || len(probe.Path) > 256) {
			return fmt.Errorf("http probe requires a bounded absolute path and http component")
		}
	}
	return nil
}

// Order returns a deterministic topological order of prerequisite dependencies.
// Communication edges are deliberately excluded, so services can communicate cyclically.
func (app Application) Order() ([]string, error) {
	names := make([]string, 0, len(app.Components))
	for name := range app.Components {
		names = append(names, name)
	}
	slices.Sort(names)
	state := map[string]int{}
	order := make([]string, 0, len(names))
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 1 {
			return fmt.Errorf("prerequisite cycle at %s", name)
		}
		if state[name] == 2 {
			return nil
		}
		state[name] = 1
		for _, dependency := range app.Components[name].DependsOn {
			if _, exists := app.Components[dependency]; !exists {
				return fmt.Errorf("missing prerequisite %s", dependency)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		order = append(order, name)
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// Validate requires a complete immutable release and verifiable provenance material.
// Signature and OIDC trust checks happen in admission, not in this structural validator.
func (request SubmitRelease) Validate() error {
	if request.APIVersion != APIVersion || !ValidName(request.ProjectID) {
		return fmt.Errorf("release requires supported apiVersion and valid projectId")
	}
	if err := request.Definition.Validate(); err != nil {
		return err
	}
	if len(request.Images) != len(request.Definition.Components) {
		return fmt.Errorf("release requires a complete component image map")
	}
	for name := range request.Definition.Components {
		if !digestPattern.MatchString(request.Images[name]) || strings.Contains(request.Images[name], "..") {
			return fmt.Errorf("component %s requires an immutable sha256 image reference", name)
		}
	}
	source := request.Source
	if source.RepositoryID <= 0 || source.OwnerID <= 0 || source.RunID <= 0 || source.RunAttempt <= 0 ||
		!shaPattern.MatchString(source.Commit) || (source.HeadCommit != "" && !shaPattern.MatchString(source.HeadCommit)) {
		return fmt.Errorf("source requires stable repository/owner/run identity and exact commit")
	}
	if len(strings.Split(source.Repository, "/")) != 2 || source.WorkflowRef == "" || source.Ref == "" {
		return fmt.Errorf("source requires repository, ref and workflow identity")
	}
	if source.Event != "push" && source.Event != "pull_request" && source.Event != "workflow_dispatch" {
		return fmt.Errorf("unsupported CI event %q", source.Event)
	}
	if (source.Event == "pull_request") != (source.PullRequest > 0) {
		return fmt.Errorf("pull_request event requires its exact pull request number")
	}
	if len(request.Provenance) == 0 || len(request.Provenance) > 256*1024 {
		return fmt.Errorf("release requires bounded signed provenance")
	}
	return nil
}
