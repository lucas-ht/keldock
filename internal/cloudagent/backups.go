package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var cnpgClusters = schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}

func (driver *Driver) backup(ctx context.Context, policy cloud.Policy, conn cloud.Connection, item cloud.Intent, items map[string]cloud.Intent) cloud.Evidence {
	binding := *item.Backup
	target, ok := policy.Databases[binding.DatabaseID]
	resource := items[binding.ResourceID]
	if !ok || target != binding.Database || resource.Resource == nil || resource.Resource.Kind != cloud.BackupStore || resource.Resource.ConnectionGeneration != conn.Generation || resource.Resource.ConnectionID != conn.ID {
		return evidence("failed", "BackupOutsideLocalPolicy", conn.Generation)
	}
	source, err := driver.Dynamic.Resource(cnpgClusters).Namespace(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	if string(source.GetUID()) != target.UID {
		return evidence("failed", "BackupSourceIdentityChanged", conn.Generation)
	}
	image, _, _ := unstructured.NestedString(source.Object, "spec", "imageName")
	if image != target.Image {
		return evidence("failed", "BackupSourceImageRequiresReview", conn.Generation)
	}
	configuration, err := driver.Probe.Config(ctx, conn, policy.Identities[conn.IdentityRef], "iam")
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	profile := policy.Profiles[resource.Resource.Profile]
	active := item.Action == "reconcile"
	writerSA, _ := cloud.WorkloadIdentity(conn, profile, item.ID, "writer")
	restoreSA, _ := cloud.WorkloadIdentity(conn, profile, item.ID, "restore")
	writer := crossplane.ConsumerRole{ID: writerSA, ARN: binding.WriterRoleARN, Namespace: target.Namespace, ServiceAccount: target.Name, Profile: resource.Resource.Profile}
	reader := crossplane.ConsumerRole{ID: restoreSA, ARN: binding.RestoreRoleARN, Namespace: target.RestoreNamespace, ServiceAccount: binding.RestoreName, Profile: resource.Resource.Profile}
	if active {
		writer.Grants = []crossplane.Grant{{Bucket: resource.Resource.ExternalName, Prefix: binding.ArchivePrefix + "/", Access: "read-write"}}
		for _, archive := range binding.Archives {
			reader.Grants = append(reader.Grants, crossplane.Grant{Bucket: resource.Resource.ExternalName, Prefix: archive.Prefix + "/", Access: "read"})
		}
	}
	for _, role := range []crossplane.ConsumerRole{writer, reader} {
		object, trust, permissions, err := crossplane.Role(policy, conn, role)
		if err != nil {
			return evidence("failed", "BackupIdentityRejected", conn.Generation)
		}
		actual, err := driver.apply(ctx, object, projectRevision(items, policy))
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		if !ready(actual) {
			return evidence("reconciling", "BackupIdentityConverging", conn.Generation)
		}
		if err := awsprobe.Role(ctx, configuration, role.ARN, profile.BoundaryARN, trust, "keldock", permissions); err != nil {
			return unavailable(err, conn.Generation)
		}
	}
	if !active {
		return driver.disconnectBackup(ctx, conn, item, source)
	}
	objectName := fmt.Sprintf("archive-%s-g%d", cloud.ShortIdentity(item.ID), binding.ArchiveGeneration)
	store := objectStore(target.Namespace, objectName, resource.Resource.ExternalName, binding.ArchivePrefix)
	if err := driver.applyBackupObject(ctx, store, item.ID); err != nil {
		return unavailable(err, conn.Generation)
	}
	if err := driver.configureSource(ctx, source, item, objectName, conn.Region); err != nil {
		return evidence("failed", "BackupFieldOwnershipConflict", conn.Generation)
	}
	rolledOut, err := driver.backupPodReady(ctx, target, binding.WriterRoleARN)
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	if !rolledOut {
		return evidence("reconciling", "BackupIdentityAndPluginRollingOut", conn.Generation)
	}
	attempt := binding.BaseAttempt
	if attempt < 1 {
		attempt = 1
	}
	baseName := fmt.Sprintf("base-%s-g%d-r%d", cloud.ShortIdentity(item.ID), binding.ArchiveGeneration, attempt)
	base := map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Backup", "metadata": map[string]any{"name": baseName, "namespace": target.Namespace}, "spec": map[string]any{"cluster": map[string]string{"name": target.Name}, "method": "plugin", "pluginConfiguration": map[string]string{"name": "barman-cloud.cloudnative-pg.io"}}}
	if err := driver.applyBackupObject(ctx, base, item.ID); err != nil {
		return unavailable(err, conn.Generation)
	}
	schedule := map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "ScheduledBackup", "metadata": map[string]any{"name": "schedule-" + cloud.ShortIdentity(item.ID), "namespace": target.Namespace}, "spec": map[string]any{"schedule": "0 " + binding.Schedule, "backupOwnerReference": "self", "cluster": map[string]string{"name": target.Name}, "method": "plugin", "pluginConfiguration": map[string]string{"name": "barman-cloud.cloudnative-pg.io"}}}
	if err := driver.applyBackupObject(ctx, schedule, item.ID); err != nil {
		return unavailable(err, conn.Generation)
	}
	report := evidence("reconciling", "BaseBackupAndWALPending", conn.Generation)
	report.Details["archiveGeneration"] = fmt.Sprint(binding.ArchiveGeneration)
	report.Details["archivePrefix"] = binding.ArchivePrefix
	backup, err := driver.Dynamic.Resource(schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "backups"}).Namespace(target.Namespace).Get(ctx, baseName, metav1.GetOptions{})
	if err != nil {
		return report
	}
	phase, _, _ := unstructured.NestedString(backup.Object, "status", "phase")
	report.Details["baseBackup"] = phase
	wal := conditionTrue(source, "ContinuousArchiving")
	report.Details["walHealthy"] = fmt.Sprint(wal)
	if phase == "completed" && wal {
		report.State = "ready"
		report.Reason = "BaseBackupAndWALReported"
	}
	if binding.RestoreGeneration > 0 {
		done, err := driver.restore(ctx, policy, conn, item, *resource.Resource)
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		report.Details["restoreReady"] = fmt.Sprint(done)
		if !done {
			report.State = "reconciling"
			report.Reason = "IsolatedRestorePending"
		} else {
			report.Details["restoredData"] = "operator-verification-required"
		}
	}
	return report
}

func objectStore(namespace, name, bucket, prefix string) map[string]any {
	return map[string]any{"apiVersion": "barmancloud.cnpg.io/v1", "kind": "ObjectStore", "metadata": map[string]any{"name": name, "namespace": namespace}, "spec": map[string]any{"configuration": map[string]any{"destinationPath": "s3://" + bucket + "/" + strings.TrimSuffix(prefix, "/"), "s3Credentials": map[string]bool{"inheritFromIAMRole": true}, "wal": map[string]any{"compression": "gzip"}, "data": map[string]any{"compression": "gzip"}}}}
}

func conditionTrue(object *unstructured.Unstructured, kind string) bool {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, raw := range conditions {
		if condition, ok := raw.(map[string]any); ok && condition["type"] == kind && condition["status"] == "True" {
			return true
		}
	}
	return false
}

func (driver *Driver) applyBackupObject(ctx context.Context, value map[string]any, id string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, &object.Object); err != nil {
		return err
	}
	object.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "keldock-cloud", "keldock.io/backup-binding": id})
	gv := object.GroupVersionKind().GroupVersion()
	plural := map[string]string{"ObjectStore": "objectstores", "Backup": "backups", "ScheduledBackup": "scheduledbackups", "Cluster": "clusters"}[object.GetKind()]
	client := driver.Dynamic.Resource(schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: plural}).Namespace(object.GetNamespace())
	actual, err := client.Get(ctx, object.GetName(), metav1.GetOptions{})
	if err == nil && actual.GetLabels()["keldock.io/backup-binding"] != id {
		return fmt.Errorf("backup resource owner conflict")
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	_, err = client.Apply(ctx, object.GetName(), object, metav1.ApplyOptions{FieldManager: "keldock-cloud-backup"})
	return err
}
