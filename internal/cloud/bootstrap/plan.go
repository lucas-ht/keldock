// Package bootstrap prepares explicit operator-owned AWS trust roots, separately from Crossplane resources.
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Spec fixes the account, region, parent DNS authority and new installation namespace.
type Spec struct {
	v1.Scope
	InstallationID string `json:"installationId"`
	ClusterID      string `json:"clusterId"`
	Account        string `json:"account"`
	Region         string `json:"region"`
	ParentZoneID   string `json:"parentZoneId"`
	ParentDomain   string `json:"parentDomain"`
	Domain         string `json:"domain"`
	IngressIPv4    string `json:"ingressIpv4"`
	Development    bool   `json:"development"`
}

// Plan is a reviewable, credential-free input to the separately authorized bootstrap apply.
type Plan struct {
	APIVersion      string            `json:"apiVersion"`
	Spec            Spec              `json:"spec"`
	Foundation      map[string]any    `json:"foundation"`
	Identity        map[string]any    `json:"identity"`
	PublicDocuments map[string]string `json:"publicDocuments"`
}

// Build resolves both stacks and their public discovery targets without modifying AWS.
func Build(spec Spec) (Plan, error) {
	if err := spec.Validate(); err != nil {
		return Plan{}, err
	}
	return Plan{APIVersion: v1.APIVersion, Spec: spec, Foundation: foundation(spec), Identity: identityStack(spec), PublicDocuments: map[string]string{
		"discovery": spec.Issuer() + "/.well-known/openid-configuration", "jwks": spec.Issuer() + "/keys.json",
	}}, nil
}

// Validate refuses apex delegation and unbounded or cross-account bootstrap configuration.
func (spec Spec) Validate() error {
	if !v1.ValidName(spec.InstallationID) || len(spec.InstallationID) > 10 || !strings.Contains(spec.ParentDomain, ".") || spec.Domain == spec.ParentDomain || !strings.HasSuffix(spec.Domain, "."+spec.ParentDomain) {
		return fmt.Errorf("bootstrap requires a short installation identity and a strict child DNS domain")
	}
	policy := spec.Policy(spec.ParentZoneID)
	if err := policy.Validate(); err != nil {
		return err
	}
	connection := cloud.Connection{CloudConnection: v1.CloudConnection{Scope: spec.Scope, ID: "bootstrap-validation", Provider: "aws", Account: spec.Account, Region: spec.Region, Profiles: []string{"private-storage", "backup-storage", "public-domain"}, IdentityRef: "aws-default"}, ClusterID: spec.ClusterID, Generation: 1, Enabled: true}
	_, err := policy.BucketName(connection, "res-validation", "private-storage")
	return err
}

// Policy materializes operator configuration only after the actual delegated zone is known.
func (spec Spec) Policy(childZoneID string) cloud.Policy {
	roles := map[string]string{}
	subjects := map[string]string{}
	for _, service := range []string{"s3", "iam", "route53"} {
		roles[service] = spec.RoleARN(service)
		subjects[service] = "provider-" + service
	}
	storage := cloud.Profile{Account: spec.Account, Kind: cloud.ObjectStorage, BucketPrefix: spec.DataPrefix(), RolePath: spec.ConsumerPath(), BoundaryARN: spec.BoundaryARN(), MaxResources: 8, ExistingBuckets: []string{spec.ExistingBucket()}}
	backup := storage
	backup.Kind = cloud.BackupStore
	backup.ExistingBuckets = nil
	return cloud.Policy{Scope: spec.Scope, ClusterID: spec.ClusterID, Accounts: []string{spec.Account}, Regions: []string{spec.Region}, Development: spec.Development, Identities: map[string]cloud.IdentityProfile{
		"aws-default": {Account: spec.Account, Issuer: spec.Issuer(), Namespace: "keldock-cloud", Roles: roles, ServiceAccounts: subjects},
	}, Profiles: map[string]cloud.Profile{
		"private-storage": storage, "backup-storage": backup,
		"public-domain": {Account: spec.Account, Kind: cloud.Domain, HostedZoneID: childZoneID, ParentDomain: spec.Domain, IngressIPv4: spec.IngressIPv4, IssuerRef: "keldock-dns01", MaxResources: 8},
	}}
}

// Fingerprint binds approval to all declared targets and exact stack policies.
func (plan Plan) Fingerprint() (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Names returns the exact two operator-owned stack names.
func (spec Spec) Names() (string, string) {
	return "keldock-" + spec.InstallationID + "-foundation", "keldock-" + spec.InstallationID + "-identity"
}

// Issuer returns the HTTPS origin containing public discovery keys, not application data.
func (spec Spec) Issuer() string {
	return "https://" + spec.IssuerBucket() + ".s3." + spec.Region + ".amazonaws.com"
}

// IssuerBucket is deliberately outside the provider's data-bucket prefix.
func (spec Spec) IssuerBucket() string {
	return "keldock-oidc-" + spec.Account + "-" + spec.InstallationID
}

// DataPrefix is the bounded namespace delegated to storage providers.
func (spec Spec) DataPrefix() string { return "keldock-" + spec.InstallationID + "-" }

// ExistingBucket names the operator-owned connect/adoption acceptance fixture.
func (spec Spec) ExistingBucket() string { return spec.DataPrefix() + "connected" }

// ConsumerPath excludes provisioning roles and their trust roots.
func (spec Spec) ConsumerPath() string { return "/keldock/" + spec.InstallationID + "/consumers/" }

// BoundaryARN is the mandatory maximum authority for generated consumer roles.
func (spec Spec) BoundaryARN() string {
	return "arn:aws:iam::" + spec.Account + ":policy/keldock/" + spec.InstallationID + "/keldock-" + spec.InstallationID + "-consumer-boundary"
}

// RoleARN resolves a service's exact operator-owned role.
func (spec Spec) RoleARN(service string) string {
	return "arn:aws:iam::" + spec.Account + ":role/keldock/" + spec.InstallationID + "/providers/keldock-" + spec.InstallationID + "-" + service
}
