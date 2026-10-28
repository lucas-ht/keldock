// Package database defines the optional PostgreSQL allocation profile without provider authority.
package database

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Image is the PostgreSQL image accepted with CNPG 1.30.1 in the first profile.
const Image = "ghcr.io/cloudnative-pg/postgresql:17.7-standard-trixie@sha256:e079dae5253fa6ed26fc65fda06d515af764dbec392dd4b46b17ab11eebd8131"

// Profile is independent operator configuration, never application-supplied infrastructure.
type Profile struct {
	StorageClass string `json:"storageClass"`
	StorageSize  string `json:"storageSize"`
}

// Policy delegates bounded allocation into one cluster for one project.
type Policy struct {
	v1.Scope
	ClusterID string                 `json:"clusterId"`
	Profiles  map[string]Profile     `json:"profiles"`
	Limit     int                    `json:"limit"`
	Backups   map[string]Destination `json:"backups,omitempty"`
}

// Validate rejects absent or unbounded operator delegation.
func (p Policy) Validate() error {
	if !v1.ValidName(p.OrganizationID) || !v1.ValidName(p.ProjectID) || !v1.ValidName(p.ClusterID) || p.Limit < 1 || p.Limit > 32 || len(p.Profiles) < 1 || len(p.Profiles) > 4 {
		return fmt.Errorf("invalid database policy")
	}
	for name, profile := range p.Profiles {
		size, err := resource.ParseQuantity(profile.StorageSize)
		if !v1.ValidName(name) || len(validation.IsDNS1123Subdomain(profile.StorageClass)) > 0 || err != nil || size.Cmp(resource.MustParse("1Gi")) < 0 || size.Cmp(resource.MustParse("100Gi")) > 0 {
			return fmt.Errorf("invalid database profile")
		}
	}
	if len(p.Backups) > 32 {
		return fmt.Errorf("backup delegation limit exceeded")
	}
	for id, destination := range p.Backups {
		if !v1.ValidName(id) || destination.validate() != nil {
			return fmt.Errorf("invalid supplied backup delegation")
		}
	}
	return nil
}

// Allocation retains data and its namespace independently of an environment's lifecycle.
type Allocation struct {
	v1.Scope
	ID            string        `json:"id"`
	EnvironmentID string        `json:"environmentId"`
	Alias         string        `json:"alias"`
	ClusterID     string        `json:"clusterId"`
	Namespace     string        `json:"namespace"`
	Profile       string        `json:"profile"`
	Configuration Profile       `json:"configuration"`
	Generation    int64         `json:"generation"`
	SourceUID     string        `json:"sourceUid,omitempty"`
	Backup        *BackupIntent `json:"backup,omitempty"`
	Action        string        `json:"action"`
	Components    []string      `json:"components"`
	Observation   *Evidence     `json:"observation,omitempty"`
}

// Evidence contains revision-aware health without credentials or SQL contents.
type Evidence struct {
	v1.Scope
	ID         string          `json:"id"`
	Generation int64           `json:"generation"`
	State      string          `json:"state"`
	Reason     string          `json:"reason"`
	UID        string          `json:"uid,omitempty"`
	ObservedAt time.Time       `json:"observedAt"`
	Backup     *BackupEvidence `json:"backup,omitempty"`
}

// ID is deterministic within an immutable environment and resource alias.
func ID(environment, alias string) string {
	return fmt.Sprintf("db-%x", sha256.Sum256([]byte(environment+"/"+alias)))[:27]
}

// Binding resolves only deterministic, nonsecret placement for an active allocation.
func (a Allocation) Binding() v1.DatabaseBinding {
	return v1.DatabaseBinding{ResourceID: a.ID, Namespace: a.Namespace, Cluster: "database", Host: "database-rw." + a.Namespace + ".svc.cluster.local", Secret: "keldock-db-" + a.Alias, Components: a.Components}
}

// ValidProjection rejects tampered namespace, scope, profile or consumer identities.
func (a Allocation) ValidProjection(p Policy) bool {
	profile, ok := p.Profiles[a.Profile]
	if !ok || a.Scope != p.Scope || a.ClusterID != p.ClusterID || a.Configuration != profile || a.ID != ID(a.EnvironmentID, a.Alias) || a.Namespace != "kd-"+strings.TrimPrefix(a.ID, "db-") || a.Generation < 1 || !v1.ValidName(a.EnvironmentID) || !v1.ValidName(a.Alias) || len(a.Components) > 16 || (a.Action != "reconcile" && a.Action != "retain") {
		return false
	}
	seen := map[string]bool{}
	for _, component := range a.Components {
		if !v1.ValidName(component) || seen[component] {
			return false
		}
		seen[component] = true
	}
	return true
}
