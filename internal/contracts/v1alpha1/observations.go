package v1alpha1

import (
	"fmt"
	"strings"
)

// Validate checks bounded observation data and complete selected-revision readiness evidence.
// The server must additionally bind its identity, generation and bundle to current desired state.
func (report Observation) Validate(definition Application) error {
	if report.Generation < 1 || !ValidName(report.EnvironmentID) || len(report.Components) > 16 || len(report.Reason) > 128 || report.ObservedAt.IsZero() {
		return fmt.Errorf("invalid observation identity or bounds")
	}
	seen := map[string]bool{}
	for _, component := range report.Components {
		if _, ok := definition.Components[component.Name]; !ok || seen[component.Name] || len(component.Message) > 1024 || len(component.Reason) > 128 || len(component.AppliedRevision) > 512 || len(component.ObjectUID) > 128 || len(component.ResourceVersion) > 128 {
			return fmt.Errorf("invalid or duplicate observed component %s", component.Name)
		}
		seen[component.Name] = true
	}
	if report.State != "ready" {
		return nil
	}
	if len(report.Components) != len(definition.Components) || !digestPattern.MatchString(report.Bundle) {
		return fmt.Errorf("ready report must cover the complete application and artifact")
	}
	_, digest, _ := strings.Cut(report.Bundle, "@")
	for _, component := range report.Components {
		if !component.Ready || component.ObjectUID == "" || component.ResourceVersion == "" || component.WorkloadUID == "" || component.WorkloadVersion == "" || !digestPattern.MatchString(component.Image) || component.ObservedGeneration < 1 || !strings.HasSuffix(component.AppliedRevision, digest) {
			return fmt.Errorf("component %s lacks selected-revision readiness evidence", component.Name)
		}
	}
	return nil
}
