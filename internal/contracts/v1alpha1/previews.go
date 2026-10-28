package v1alpha1

import "fmt"

// PreviewPolicy is operator-owned configuration, never accepted from application source.
type PreviewPolicy struct {
	Profile    string `json:"profile"`
	TTLSeconds int64  `json:"ttlSeconds"`
	MaxActive  int    `json:"maxActive"`
}

// Normalize supplies the focused stateless profile and validates its bounded limits.
func (policy PreviewPolicy) Normalize() (PreviewPolicy, error) {
	if policy.Profile == "" {
		policy.Profile = "stateless-v1"
	}
	if policy.Profile == "disabled" {
		return PreviewPolicy{Profile: "disabled"}, nil
	}
	if policy.TTLSeconds == 0 {
		policy.TTLSeconds = 86400
	}
	if policy.MaxActive == 0 {
		policy.MaxActive = 10
	}
	if policy.Profile != "stateless-v1" || policy.TTLSeconds < 60 || policy.TTLSeconds > 604800 || policy.MaxActive < 1 || policy.MaxActive > 50 {
		return policy, fmt.Errorf("unsupported preview profile or limits")
	}
	return policy, nil
}
