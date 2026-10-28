package cloud

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

var hostnameLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateConnection admits only a project's explicitly configured account, region and identities.
func (policy Policy) ValidateConnection(connection Connection) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if connection.Scope != policy.Scope || connection.ClusterID != policy.ClusterID || !v1.ValidName(connection.ID) || connection.Generation < 1 || connection.Provider != "aws" || !slices.Contains(policy.Accounts, connection.Account) || !slices.Contains(policy.Regions, connection.Region) {
		return fmt.Errorf("connection outside operator cloud policy")
	}
	identity, ok := policy.Identities[connection.IdentityRef]
	if !ok || identity.Account != connection.Account || len(connection.Profiles) == 0 || len(connection.Profiles) > 16 {
		return fmt.Errorf("unapproved cloud identity or profiles")
	}
	seen := map[string]bool{}
	for _, name := range connection.Profiles {
		if profile, ok := policy.Profiles[name]; !ok || seen[name] || profile.Account != connection.Account {
			return fmt.Errorf("unapproved or duplicate cloud profile")
		}
		seen[name] = true
	}
	return nil
}

// ValidateResource prevents application inputs from selecting arbitrary provider objects or ownership.
func (policy Policy) ValidateResource(connection Connection, request ResourceRequest) error {
	if err := policy.ValidateConnection(connection); err != nil {
		return err
	}
	profile, ok := policy.Profiles[request.Profile]
	if !connection.Enabled || !ok || !slices.Contains(connection.Profiles, request.Profile) || request.ConnectionID != connection.ID || profile.Kind != request.Kind || (request.Kind != ObjectStorage && request.Kind != BackupStore) {
		return fmt.Errorf("resource outside enabled connection and profile")
	}
	switch request.Mode {
	case "create":
		if request.ExistingName != "" {
			return fmt.Errorf("new resource names are generated; connecting does not imply adoption")
		}
	case "connect":
		if !validBucket(request.ExistingName) || !slices.Contains(profile.ExistingBuckets, request.ExistingName) {
			return fmt.Errorf("an explicitly granted existing general-purpose S3 bucket is required")
		}
	default:
		return fmt.Errorf("resource creation supports create or connect; adoption requires separate review")
	}
	return nil
}

// ValidateBinding bounds consumer grants without accepting roles, policies or namespace authority.
func ValidateBinding(request BindingRequest) error {
	if !v1.ValidName(request.ResourceID) || !v1.ValidName(request.EnvironmentID) || request.Generation < 1 || len(request.Components) < 1 || len(request.Components) > 16 || (request.Access != "read" && request.Access != "read-write") {
		return fmt.Errorf("binding requires exact resource, environment, revision and access")
	}
	seen := map[string]bool{}
	for _, component := range request.Components {
		if !v1.ValidName(component) || strings.HasPrefix(component, "keldock-") || seen[component] {
			return fmt.Errorf("invalid or duplicate binding consumer")
		}
		seen[component] = true
	}
	return nil
}

// ValidateDomain enforces operator DNS authority; record targets and certificate issuers are fixed policy.
func (policy Policy) ValidateDomain(connection Connection, request DomainRequest) error {
	if err := policy.ValidateConnection(connection); err != nil {
		return err
	}
	profile, ok := policy.Profiles[request.Profile]
	if !connection.Enabled || !ok || profile.Kind != Domain || !slices.Contains(connection.Profiles, request.Profile) || request.ConnectionID != connection.ID || !v1.ValidName(request.EnvironmentID) || !v1.ValidName(request.Component) || strings.HasPrefix(request.Component, "keldock-") || request.Generation < 1 || !validHostname(request.Hostname) {
		return fmt.Errorf("domain request outside enabled profile and exact consumer")
	}
	if request.Hostname != profile.ParentDomain && !strings.HasSuffix(request.Hostname, "."+profile.ParentDomain) {
		return fmt.Errorf("hostname outside operator-owned parent domain")
	}
	return nil
}

func validHostname(value string) bool {
	if len(value) < 3 || len(value) > 253 || strings.ToLower(value) != value || strings.Contains(value, "*") || strings.HasSuffix(value, ".") {
		return false
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	_, err := netip.ParseAddr(value)
	return err != nil
}

func validBucketPrefix(value string) bool {
	return len(value) >= 8 && len(value) <= 32 && strings.HasPrefix(value, "keldock-") && strings.HasSuffix(value, "-") && !strings.Contains(value, ".") && validBucket(value+"resource")
}

func validBucket(value string) bool {
	if len(value) < 3 || len(value) > 63 || strings.ToLower(value) != value || strings.Contains(value, "..") || strings.Contains(value, ".-") || strings.Contains(value, "-.") {
		return false
	}
	for _, prefix := range []string{"xn--", "sthree-", "amzn-s3-demo-"} {
		if strings.HasPrefix(value, prefix) {
			return false
		}
	}
	for _, suffix := range []string{"-s3alias", "--ol-s3", ".mrap", "--x-s3", "--table-s3"} {
		if strings.HasSuffix(value, suffix) {
			return false
		}
	}
	for _, label := range strings.Split(value, ".") {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	_, err := netip.ParseAddr(value)
	return err != nil
}
