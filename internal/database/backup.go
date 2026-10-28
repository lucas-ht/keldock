package database

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// SecretReference pins an operator-supplied identity without carrying its value.
type SecretReference struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

// Destination is an independently approved, supplied S3-compatible backup target.
// It neither creates a bucket nor grants cloud/provider administration.
type Destination struct {
	AllocationID  string           `json:"allocationId"`
	SourceUID     string           `json:"sourceUid"`
	Description   string           `json:"description"`
	Endpoint      string           `json:"endpoint"`
	ProbeEndpoint string           `json:"probeEndpoint,omitempty"`
	Region        string           `json:"region"`
	Bucket        string           `json:"bucket"`
	Writer        SecretReference  `json:"writer"`
	Reader        SecretReference  `json:"reader"`
	CA            *SecretReference `json:"ca,omitempty"`
	Development   bool             `json:"development,omitempty"`
}

func (d Destination) validate() error {
	if !v1.ValidName(d.AllocationID) || d.SourceUID == "" || len(d.SourceUID) > 80 || len(d.Description) > 160 || len(validation.IsDNS1123Subdomain(d.Bucket)) > 0 || d.Region == "" || len(d.Region) > 32 || d.Writer == d.Reader {
		return fmt.Errorf("invalid supplied backup target")
	}
	for _, ref := range []SecretReference{d.Writer, d.Reader} {
		if len(validation.IsDNS1123Subdomain(ref.Namespace)) > 0 || len(validation.IsDNS1123Subdomain(ref.Name)) > 0 || ref.UID == "" || len(ref.UID) > 80 {
			return fmt.Errorf("backup identities require exact private Secret references")
		}
	}
	for index, endpoint := range []string{d.Endpoint, d.ProbeEndpoint} {
		if index == 1 && endpoint == "" {
			continue
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || (parsed.Scheme != "https" && (!d.Development || parsed.Scheme != "http")) {
			return fmt.Errorf("backup endpoints require HTTPS or explicit development transport")
		}
	}
	if d.ProbeEndpoint != "" && d.ProbeEndpoint != d.Endpoint && !d.Development {
		return fmt.Errorf("separate probe transport is development-only")
	}
	if d.CA != nil {
		ref := *d.CA
		if len(validation.IsDNS1123Subdomain(ref.Namespace)) > 0 || len(validation.IsDNS1123Subdomain(ref.Name)) > 0 || ref.UID == "" {
			return fmt.Errorf("invalid backup TLS authority")
		}
	}
	return nil
}

// ArchivePrefix is immutable for a source identity; restored clusters never write it.
func ArchivePrefix(scope v1.Scope, id string) string {
	return "keldock/" + scope.OrganizationID + "/" + scope.ProjectID + "/" + id + "/generation-1"
}

// BackupIntent selects finite base-backup and isolated-restore attempts.
type BackupIntent struct {
	DestinationID  string `json:"destinationId"`
	SourceUID      string `json:"sourceUid"`
	BaseAttempt    int    `json:"baseAttempt"`
	RestoreAttempt int    `json:"restoreAttempt"`
	Paused         bool   `json:"paused"`
}

// BackupEvidence distinguishes base/WAL protection from an observed isolated restore.
type BackupEvidence struct {
	State            string     `json:"state"`
	Reason           string     `json:"reason"`
	BaseBackup       string     `json:"baseBackup,omitempty"`
	WALObjects       bool       `json:"walObjects"`
	WALHealthy       bool       `json:"walHealthy"`
	LastArchivedAt   *time.Time `json:"lastArchivedAt,omitempty"`
	LastBaseBackupAt *time.Time `json:"lastBaseBackupAt,omitempty"`
	RestoreNamespace string     `json:"restoreNamespace,omitempty"`
	RestoreName      string     `json:"restoreName,omitempty"`
	RestoreUID       string     `json:"restoreUid,omitempty"`
	RestoreReady     bool       `json:"restoreReady"`
}

// Valid rejects unbounded or credential-shaped agent data at the reporting boundary.
func (e BackupEvidence) Valid() bool {
	if e.State != "pending" && e.State != "ready" && e.State != "failed" && e.State != "paused" {
		return false
	}
	if len(e.Reason) > 160 || strings.Trim(e.Reason, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_") != "" {
		return false
	}
	for _, name := range []string{e.BaseBackup, e.RestoreNamespace, e.RestoreName} {
		if name != "" && (!v1.ValidName(name) || len(name) > 80) {
			return false
		}
	}
	// Kubernetes UIDs are opaque identities, commonly UUIDs beginning with a
	// digit. They are not product names, which deliberately start with a letter.
	if len(e.RestoreUID) > 80 || strings.Trim(e.RestoreUID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return false
	}
	for _, at := range []*time.Time{e.LastArchivedAt, e.LastBaseBackupAt} {
		if at != nil && (at.IsZero() || at.After(time.Now().Add(time.Minute))) {
			return false
		}
	}
	return true
}

// RestoreNamespace is separate from both source and disposable application ownership.
func RestoreNamespace(id string, attempt int) string {
	return fmt.Sprintf("kr-%s-%d", strings.TrimPrefix(id, "db-"), attempt)
}
