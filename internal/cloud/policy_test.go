package cloud

import (
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func fixturePolicy() (Policy, Connection) {
	scope := v1.Scope{OrganizationID: "test-org", ProjectID: "sample"}
	identity := IdentityProfile{Account: "123456789012", Issuer: "https://s3.eu-central-1.amazonaws.com/keldock-issuer", Namespace: "keldock-cloud", ServiceAccounts: map[string]string{}, Roles: map[string]string{}}
	for _, service := range []string{"s3", "iam", "route53"} {
		identity.ServiceAccounts[service] = "provider-" + service
		identity.Roles[service] = "arn:aws:iam::123456789012:role/keldock/test/providers/" + service
	}
	storage := Profile{Account: identity.Account, Kind: ObjectStorage, BucketPrefix: "keldock-test-", RolePath: "/keldock/test/consumers/", BoundaryARN: "arn:aws:iam::123456789012:policy/keldock/test/boundary", MaxResources: 10, ExistingBuckets: []string{"customer-approved-store"}}
	backup := storage
	backup.Kind = BackupStore
	policy := Policy{Scope: scope, ClusterID: "test-cluster", Accounts: []string{identity.Account}, Regions: []string{"eu-central-1"}, Identities: map[string]IdentityProfile{"test-identity": identity}, Profiles: map[string]Profile{
		"private-storage": storage, "backup-storage": backup,
		"public-domain": {Account: identity.Account, Kind: Domain, HostedZoneID: "Z123456789", ParentDomain: "test.example.com", IngressIPv4: "192.0.2.10", IssuerRef: "test-acme", MaxResources: 10},
	}}
	connection := Connection{CloudConnection: v1.CloudConnection{Scope: scope, ID: "connection-one", Provider: "aws", Account: identity.Account, Region: "eu-central-1", Profiles: []string{"private-storage", "backup-storage", "public-domain"}, IdentityRef: "test-identity"}, ClusterID: policy.ClusterID, Generation: 1, Enabled: true}
	return policy, connection
}

func TestConnectionUsesOnlyOperatorAuthority(t *testing.T) {
	policy, connection := fixturePolicy()
	if err := policy.ValidateConnection(connection); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Connection){
		"foreign tenant":    func(c *Connection) { c.OrganizationID = "other-org" },
		"foreign project":   func(c *Connection) { c.ProjectID = "other-project" },
		"foreign cluster":   func(c *Connection) { c.ClusterID = "other-cluster" },
		"foreign account":   func(c *Connection) { c.Account = "999999999999" },
		"unapproved region": func(c *Connection) { c.Region = "us-east-1" },
		"unknown identity":  func(c *Connection) { c.IdentityRef = "administrator" },
		"unknown profile":   func(c *Connection) { c.Profiles = []string{"administrator"} },
		"duplicate profile": func(c *Connection) { c.Profiles = []string{"private-storage", "private-storage"} },
		"no generation":     func(c *Connection) { c.Generation = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := connection
			mutate(&candidate)
			if policy.ValidateConnection(candidate) == nil {
				t.Fatal("unapproved authority accepted")
			}
		})
	}
}

func TestPolicyRejectsUnboundedIdentityAndProviderTargets(t *testing.T) {
	tests := map[string]func(*Policy){
		"wildcard role":           func(p *Policy) { p.Identities["test-identity"].Roles["s3"] = "arn:aws:iam::123456789012:role/*" },
		"foreign role":            func(p *Policy) { p.Identities["test-identity"].Roles["s3"] = "arn:aws:iam::999999999999:role/other" },
		"shared provider subject": func(p *Policy) { p.Identities["test-identity"].ServiceAccounts["iam"] = "provider-s3" },
		"shared provider role": func(p *Policy) {
			p.Identities["test-identity"].Roles["iam"] = p.Identities["test-identity"].Roles["s3"]
		},
		"internal issuer": func(p *Policy) {
			x := p.Identities["test-identity"]
			x.Issuer = "https://127.0.0.1"
			p.Identities["test-identity"] = x
		},
		"issuer credentials": func(p *Policy) {
			x := p.Identities["test-identity"]
			x.Issuer = "https://user:password@issuer.example.com"
			p.Identities["test-identity"] = x
		},
		"issuer query": func(p *Policy) {
			x := p.Identities["test-identity"]
			x.Issuer = "https://issuer.example.com?"
			p.Identities["test-identity"] = x
		},
		"issuer traversal": func(p *Policy) {
			x := p.Identities["test-identity"]
			x.Issuer = "https://issuer.example.com/../other"
			p.Identities["test-identity"] = x
		},
		"unbounded bucket prefix": func(p *Policy) {
			x := p.Profiles["private-storage"]
			x.BucketPrefix = "*"
			p.Profiles["private-storage"] = x
		},
		"foreign boundary": func(p *Policy) {
			x := p.Profiles["private-storage"]
			x.BoundaryARN = "arn:aws:iam::999999999999:policy/keldock/test/boundary"
			p.Profiles["private-storage"] = x
		},
		"role traversal": func(p *Policy) {
			x := p.Profiles["private-storage"]
			x.RolePath = "/keldock/../admin/"
			p.Profiles["private-storage"] = x
		},
		"role control character": func(p *Policy) {
			x := p.Profiles["private-storage"]
			x.RolePath = "/keldock/test\n/"
			p.Profiles["private-storage"] = x
		},
		"wildcard existing grant": func(p *Policy) {
			x := p.Profiles["private-storage"]
			x.ExistingBuckets = []string{"*"}
			p.Profiles["private-storage"] = x
		},
		"unlimited capacity": func(p *Policy) { x := p.Profiles["public-domain"]; x.MaxResources = 0; p.Profiles["public-domain"] = x },
		"wildcard domain": func(p *Policy) {
			x := p.Profiles["public-domain"]
			x.ParentDomain = "*.example.com"
			p.Profiles["public-domain"] = x
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			policy, _ := fixturePolicy()
			mutate(&policy)
			if policy.Validate() == nil {
				t.Fatal("unsafe operator policy accepted")
			}
		})
	}
}
