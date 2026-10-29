package cluster

import (
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestDependencyReadinessUsesCurrentPrerequisites(t *testing.T) {
	definition := v1.Application{Components: map[string]v1.Component{
		"api": {}, "migrate": {DependsOn: []string{"api"}},
		"web":    {DependsOn: []string{"api", "migrate"}},
		"worker": {DependsOn: []string{"web"}}, "schedule": {DependsOn: []string{"api"}},
	}}
	for _, test := range []struct {
		name                              string
		jobReady, omitJob, consumersReady bool
	}{
		{"completed job", true, false, true},
		{"failed job despite stale controller readiness", false, false, false},
		{"missing selected job", false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			health := []v1.ComponentHealth{{Name: "worker", Ready: true}, {Name: "web", Ready: true}, {Name: "api", Ready: true}, {Name: "schedule", Ready: true}}
			if !test.omitJob {
				health = append(health, v1.ComponentHealth{Name: "migrate", Ready: test.jobReady, Reason: "SelectedJobEvidence"})
			}
			if err := constrainDependencyReadiness(definition, health); err != nil {
				t.Fatal(err)
			}
			for _, component := range health {
				if component.Name == "web" || component.Name == "worker" {
					if component.Ready != test.consumersReady || (!component.Ready && component.Reason != "DependencyNotReady") {
						t.Fatal("dependent retained stale selected-release readiness", component.Name)
					}
				} else if component.Name == "api" || component.Name == "schedule" {
					if !component.Ready {
						t.Fatal("an independent healthy branch was blocked")
					}
				} else if component.Reason != "SelectedJobEvidence" {
					t.Fatal("prerequisite evidence was overwritten")
				}
			}
		})
	}
}
