package v1alpha1

import "fmt"

// ResourceRequest selects a bounded operator profile, never a cluster or Secret.
type ResourceRequest struct {
	Kind    string `json:"kind" yaml:"kind"`
	Profile string `json:"profile" yaml:"profile"`
}

// DatabaseBinding is trusted placement. Passwords never enter a release or bundle.
type DatabaseBinding struct {
	ResourceID string   `json:"resourceId"`
	Namespace  string   `json:"namespace"`
	Cluster    string   `json:"cluster"`
	Host       string   `json:"host"`
	Secret     string   `json:"secret"`
	Components []string `json:"components"`
}

func (app Application) validateResources() error {
	if len(app.Resources) > 4 {
		return fmt.Errorf("at most four application resources are supported")
	}
	for name, request := range app.Resources {
		if !ValidName(name) || request.Kind != "postgresql" || !ValidName(request.Profile) {
			return fmt.Errorf("invalid PostgreSQL resource request")
		}
	}
	for name, component := range app.Components {
		if len(component.Resources) > 1 {
			return fmt.Errorf("component %s supports one PostgreSQL binding", name)
		}
		for _, resource := range component.Resources {
			if _, ok := app.Resources[resource]; !ok {
				return fmt.Errorf("component %s refers to unknown resource %s", name, resource)
			}
		}
	}
	return nil
}
