package cloud

import (
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

var (
	accountPattern  = regexp.MustCompile(`^[0-9]{12}$`)
	regionPattern   = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[1-9][0-9]?$`)
	zonePattern     = regexp.MustCompile(`^Z[A-Z0-9]{1,31}$`)
	rolePattern     = regexp.MustCompile(`^arn:aws:iam::([0-9]{12}):role/([a-zA-Z0-9+=,.@_/-]+)$`)
	rolePathPattern = regexp.MustCompile(`^/keldock/[a-z0-9][a-z0-9/-]*/$`)
	boundaryPattern = regexp.MustCompile(`^arn:aws:iam::([0-9]{12}):policy/keldock/[a-zA-Z0-9+=,.@_/-]+$`)
)

// IdentityProfile names operator-supplied short-lived trust, never an access-key pair.
type IdentityProfile struct {
	Account         string            `json:"account"`
	Issuer          string            `json:"issuer"`
	Namespace       string            `json:"namespace"`
	ServiceAccounts map[string]string `json:"serviceAccounts"`
	Roles           map[string]string `json:"roles"`
}

// Profile is a fixed approved implementation with explicit placement and capacity.
type Profile struct {
	Account         string   `json:"account"`
	Kind            string   `json:"kind"`
	BucketPrefix    string   `json:"bucketPrefix,omitempty"`
	RolePath        string   `json:"rolePath,omitempty"`
	BoundaryARN     string   `json:"boundaryArn,omitempty"`
	HostedZoneID    string   `json:"hostedZoneId,omitempty"`
	ParentDomain    string   `json:"parentDomain,omitempty"`
	IngressIPv4     string   `json:"ingressIpv4,omitempty"`
	IssuerRef       string   `json:"issuerRef,omitempty"`
	MaxResources    int      `json:"maxResources"`
	ExistingBuckets []string `json:"existingBuckets,omitempty"`
}

// Policy is private installation configuration. Application requests cannot replace it.
type Policy struct {
	v1.Scope
	ClusterID   string                     `json:"clusterId"`
	Accounts    []string                   `json:"accounts"`
	Regions     []string                   `json:"regions"`
	Identities  map[string]IdentityProfile `json:"identities"`
	Profiles    map[string]Profile         `json:"profiles"`
	Databases   map[string]DatabaseTarget  `json:"databases,omitempty"`
	EgressCIDRs []string                   `json:"egressCidrs,omitempty"`
	Development bool                       `json:"development"`
}

// Validate checks authority boundaries before enabling this optional profile.
func (policy Policy) Validate() error {
	if !v1.ValidName(policy.OrganizationID) || !v1.ValidName(policy.ProjectID) || !v1.ValidName(policy.ClusterID) || len(policy.Accounts) == 0 || len(policy.Accounts) > 16 || len(policy.Regions) == 0 || len(policy.Regions) > 16 || len(policy.Identities) == 0 || len(policy.Profiles) == 0 || len(policy.Identities) > 16 || len(policy.Profiles) > 16 {
		return fmt.Errorf("cloud policy requires bounded ownership, accounts, regions, identities and profiles")
	}
	for _, account := range policy.Accounts {
		if !accountPattern.MatchString(account) {
			return fmt.Errorf("invalid AWS account")
		}
	}
	for _, region := range policy.Regions {
		if !validRegion(region) {
			return fmt.Errorf("unsupported AWS region")
		}
	}
	if len(policy.Databases) > 16 || len(policy.EgressCIDRs) > 128 {
		return fmt.Errorf("cloud database and network grants must be bounded")
	}
	for name, database := range policy.Databases {
		if !v1.ValidName(name) || !v1.ValidName(database.Namespace) || !v1.ValidName(database.Name) || len(database.UID) < 8 || len(database.UID) > 64 {
			return fmt.Errorf("backup source requires an exact operator-approved database identity")
		}
	}
	for _, cidr := range policy.EgressCIDRs {
		if !publicCIDR(cidr) {
			return fmt.Errorf("AWS data egress requires explicit public IPv4 CIDRs")
		}
	}
	for name, identity := range policy.Identities {
		if !v1.ValidName(name) || !slices.Contains(policy.Accounts, identity.Account) || !validIssuer(identity.Issuer) || !v1.ValidName(identity.Namespace) || len(identity.Roles) != 3 || len(identity.ServiceAccounts) != 3 {
			return fmt.Errorf("invalid operator identity profile")
		}
		subjects := map[string]bool{}
		roles := map[string]bool{}
		for _, service := range []string{"s3", "iam", "route53"} {
			subject := identity.ServiceAccounts[service]
			if !v1.ValidName(subject) || subjects[subject] || roles[identity.Roles[service]] || !validRole(identity.Roles[service], identity.Account) {
				return fmt.Errorf("cloud providers require distinct exact service accounts and matching-account roles")
			}
			subjects[subject] = true
			roles[identity.Roles[service]] = true
		}
	}
	for name, profile := range policy.Profiles {
		if !v1.ValidName(name) || !slices.Contains(policy.Accounts, profile.Account) || profile.MaxResources < 1 || profile.MaxResources > 50 || len(profile.ExistingBuckets) > 50 {
			return fmt.Errorf("invalid cloud profile or capacity")
		}
		switch profile.Kind {
		case ObjectStorage, BackupStore:
			if !validBucketPrefix(profile.BucketPrefix) || !validRolePath(profile.RolePath) || !validBoundary(profile.BoundaryARN, []string{profile.Account}) || profile.HostedZoneID != "" || profile.ParentDomain != "" || profile.IngressIPv4 != "" || profile.IssuerRef != "" {
				return fmt.Errorf("storage profile requires a bounded bucket prefix, consumer role path and permissions boundary")
			}
			seen := map[string]bool{}
			for _, bucket := range profile.ExistingBuckets {
				if !validBucket(bucket) || seen[bucket] {
					return fmt.Errorf("existing bucket grants must be exact and unique")
				}
				seen[bucket] = true
			}
		case Domain:
			ip, err := netip.ParseAddr(profile.IngressIPv4)
			if err != nil || !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() || (!policy.Development && ip.IsLoopback()) || !zonePattern.MatchString(profile.HostedZoneID) || !validHostname(profile.ParentDomain) || !v1.ValidName(profile.IssuerRef) || profile.BucketPrefix != "" || profile.RolePath != "" || profile.BoundaryARN != "" || len(profile.ExistingBuckets) > 0 {
				return fmt.Errorf("domain profile requires an exact zone, parent domain, ingress address and issuer")
			}
		default:
			return fmt.Errorf("unsupported cloud profile kind")
		}
	}
	return nil
}

func publicCIDR(cidr string) bool {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 8 || prefix.Masked() != prefix {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/3"} {
		if prefix.Overlaps(netip.MustParsePrefix(raw)) {
			return false
		}
	}
	return true
}

func validRegion(value string) bool {
	return regionPattern.MatchString(value) && !strings.HasPrefix(value, "cn-") && !strings.HasPrefix(value, "us-gov-") && !strings.HasPrefix(value, "us-iso")
}

func validRole(value, account string) bool {
	match := rolePattern.FindStringSubmatch(value)
	if len(match) != 3 || match[1] != account || len(match[2]) > 512 || strings.Contains(match[2], "//") || strings.Contains(match[2], "..") {
		return false
	}
	name := match[2][strings.LastIndex(match[2], "/")+1:]
	return len(name) > 0 && len(name) <= 64
}

func validRolePath(path string) bool {
	return len(path) <= 128 && rolePathPattern.MatchString(path) && !strings.Contains(path, "//")
}

func validBoundary(value string, accounts []string) bool {
	match := boundaryPattern.FindStringSubmatch(value)
	return len(match) == 2 && slices.Contains(accounts, match[1]) && len(value) <= 256 && !strings.Contains(value, "..") && !strings.Contains(value, "//") && !strings.HasSuffix(value, "/")
}

func validIssuer(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && validHostname(parsed.Hostname()) && parsed.User == nil && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && parsed.RawPath == "" && parsed.Port() == "" && !strings.HasSuffix(raw, "/") && !strings.ContainsAny(raw, "* \\#") && (parsed.Path == "" || path.Clean(parsed.Path) == parsed.Path)
}
