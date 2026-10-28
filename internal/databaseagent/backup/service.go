package backup

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/provenance"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Service owns native archive projection, scoped credentials and isolated restore.
type Service struct {
	Client           dynamic.Interface
	apply            func(context.Context, database.Allocation, map[string]any, string, string) (*unstructured.Unstructured, error)
	cluster, network func(database.Allocation) map[string]any
	permissionMu     sync.Mutex
	permissions      map[string]permissionCheck
}
type permissionCheck struct {
	Hash string
	At   time.Time
}

// New keeps cluster ownership enforcement in the provisioner's injected apply boundary.
func New(client dynamic.Interface, apply func(context.Context, database.Allocation, map[string]any, string, string) (*unstructured.Unstructured, error), cluster, network func(database.Allocation) map[string]any) *Service {
	return &Service{Client: client, apply: apply, cluster: cluster, network: network}
}

var resources = map[string]schema.GroupVersionResource{
	"Cluster": {Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"},
	"Backup":  {Group: "postgresql.cnpg.io", Version: "v1", Resource: "backups"},
	"Secret":  {Version: "v1", Resource: "secrets"},
	"Pod":     {Version: "v1", Resource: "pods"},
}

// Reconcile reports protection and isolated restore separately from source availability.
func (d *Service) Reconcile(ctx context.Context, p database.Policy, a database.Allocation) *database.BackupEvidence {
	e := &database.BackupEvidence{State: "pending", Reason: "BaseBackupAndWALPending"}
	destination, ok := p.Backups[a.Backup.DestinationID]
	if !ok || destination.AllocationID != a.ID || destination.SourceUID != a.SourceUID || a.Backup.SourceUID != a.SourceUID || a.Backup.BaseAttempt < 1 || a.Backup.BaseAttempt > 10 || a.Backup.RestoreAttempt < 0 || a.Backup.RestoreAttempt > 4 {
		e.State = "failed"
		e.Reason = "BackupOutsideOperatorDelegation"
		return e
	}
	source, err := d.Client.Resource(resources["Cluster"]).Namespace(a.Namespace).Get(ctx, "database", metav1.GetOptions{})
	if err != nil || string(source.GetUID()) != destination.SourceUID {
		e.State = "failed"
		e.Reason = "BackupSourceIdentityChanged"
		return e
	}
	if a.Backup.Paused {
		if err := d.configureBackupSource(ctx, a, source, true); err != nil {
			e.State = "failed"
			e.Reason = "BackupOwnershipConflict"
			return e
		}
		if _, err := d.backupApply(ctx, a, schedule(a, true)); err != nil {
			e.State = "failed"
			e.Reason = "BackupScheduleUnavailable"
			return e
		}
		e.State = "paused"
		e.Reason = "ArchiveRetainedBackupsPaused"
		return e
	}
	writer, writerValues, err := d.s3(ctx, destination, false)
	if err != nil {
		e.State = "failed"
		e.Reason = "BackupWriterUnavailable"
		return e
	}
	defer writer.Options().HTTPClient.(*http.Client).CloseIdleConnections()
	reader, readerValues, err := d.s3(ctx, destination, true)
	if err != nil {
		e.State = "failed"
		e.Reason = "RestoreIdentityUnavailable"
		return e
	}
	defer reader.Options().HTTPClient.(*http.Client).CloseIdleConnections()
	fingerprint, err := provenance.Hash(struct {
		Generation     int64
		Destination    database.Destination
		Writer, Reader map[string]string
	}{a.Generation, destination, writerValues, readerValues})
	if err != nil {
		e.State = "failed"
		e.Reason = "BackupPolicyInvalid"
		return e
	}
	key := a.OrganizationID + "/" + a.ProjectID + "/" + a.ID
	d.permissionMu.Lock()
	cached := d.permissions[key]
	d.permissionMu.Unlock()
	if cached.Hash != fingerprint || time.Since(cached.At) > 2*time.Minute {
		if err := probeBackupPermissions(ctx, writer, reader, a, destination); err != nil {
			e.State = "failed"
			e.Reason = "ScopedBackupIdentityCheckFailed"
			return e
		}
		d.permissionMu.Lock()
		if d.permissions == nil {
			d.permissions = map[string]permissionCheck{}
		}
		d.permissions[key] = permissionCheck{Hash: fingerprint, At: time.Now()}
		d.permissionMu.Unlock()
	}
	if err := d.projectBackupSecret(ctx, a, a.Namespace, "archive-writer", writerValues); err != nil {
		e.State = "failed"
		e.Reason = "BackupCredentialProjectionBlocked"
		return e
	}
	if destination.CA != nil {
		ca, err := d.secret(ctx, *destination.CA)
		if err != nil {
			e.State = "failed"
			e.Reason = "BackupTLSAuthorityUnavailable"
			return e
		}
		if err := d.projectBackupSecret(ctx, a, a.Namespace, "archive-ca", map[string]string{"ca.crt": ca["ca.crt"]}); err != nil {
			e.State = "failed"
			e.Reason = "BackupTLSProjectionBlocked"
			return e
		}
	}
	if _, err := d.backupApply(ctx, a, archiveObject(a, destination, a.Namespace, "archive-writer")); err != nil {
		e.State = "failed"
		e.Reason = "BackupPluginUnavailable"
		return e
	}
	if err := d.configureBackupSource(ctx, a, source, false); err != nil {
		e.State = "failed"
		e.Reason = "BackupOwnershipConflict"
		return e
	}
	if !d.backupPodReady(ctx, a) {
		e.Reason = "BackupSidecarRollingOut"
		return e
	}
	name := fmt.Sprintf("base-%d", a.Backup.BaseAttempt)
	base := map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Backup", "metadata": map[string]any{"name": name, "namespace": a.Namespace}, "spec": map[string]any{"cluster": map[string]string{"name": "database"}, "method": "plugin", "pluginConfiguration": map[string]string{"name": "barman-cloud.cloudnative-pg.io"}}}
	if _, err := d.backupApply(ctx, a, base); err != nil {
		e.State = "failed"
		e.Reason = "BaseBackupRequestBlocked"
		return e
	}
	if _, err := d.backupApply(ctx, a, schedule(a, false)); err != nil {
		e.State = "failed"
		e.Reason = "BackupScheduleBlocked"
		return e
	}
	backup, err := d.Client.Resource(resources["Backup"]).Namespace(a.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return e
	}
	e.BaseBackup, _, _ = unstructured.NestedString(backup.Object, "status", "phase")
	if stopped, _, _ := unstructured.NestedString(backup.Object, "status", "stoppedAt"); stopped != "" {
		if at, err := time.Parse(time.RFC3339, stopped); err == nil {
			e.LastBaseBackupAt = &at
		}
	}
	e.WALHealthy = condition(source, "ContinuousArchiving")
	e.WALObjects, e.LastArchivedAt, err = archivedWAL(ctx, writer, a, destination, backup)
	if err != nil {
		e.State = "failed"
		e.Reason = "ArchiveObservationUnavailable"
		return e
	}
	if e.BaseBackup == "failed" {
		e.State = "failed"
		e.Reason = "BaseBackupFailed_RetryRequired"
	}
	if e.BaseBackup == "completed" && e.WALHealthy && e.WALObjects {
		e.State = "ready"
		e.Reason = "BaseBackupAndWALObserved"
	}
	if a.Backup.RestoreAttempt > 0 {
		e.RestoreNamespace = database.RestoreNamespace(a.ID, a.Backup.RestoreAttempt)
		e.RestoreName = "restore"
		actual, err := d.restoreBackup(ctx, a, destination, readerValues)
		if err != nil {
			e.State = "failed"
			e.Reason = "IsolatedRestoreBlocked"
			return e
		}
		e.RestoreUID = string(actual.GetUID())
		e.RestoreReady = condition(actual, "Ready")
		if !e.RestoreReady {
			e.Reason = "IsolatedRestorePending"
		}
	}
	return e
}

func schedule(a database.Allocation, paused bool) map[string]any {
	return map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "ScheduledBackup", "metadata": map[string]any{"name": "daily", "namespace": a.Namespace}, "spec": map[string]any{"cluster": map[string]string{"name": "database"}, "schedule": "0 0 3 * * *", "suspend": paused, "backupOwnerReference": "none", "method": "plugin", "pluginConfiguration": map[string]string{"name": "barman-cloud.cloudnative-pg.io"}}}
}

func condition(object *unstructured.Unstructured, kind string) bool {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, raw := range conditions {
		value, ok := raw.(map[string]any)
		if ok && value["type"] == kind && value["status"] == "True" {
			return true
		}
	}
	return false
}
