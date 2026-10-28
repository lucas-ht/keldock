package cloudagent

import (
	"context"
	"fmt"

	"github.com/lucas-ht/keldock/internal/cloud"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func awsEnvironment(arn, region string) []any {
	return []any{map[string]string{"name": "AWS_ROLE_ARN", "value": arn}, map[string]string{"name": "AWS_WEB_IDENTITY_TOKEN_FILE", "value": "/projected/aws/token"}, map[string]string{"name": "AWS_REGION", "value": region}, map[string]string{"name": "AWS_DEFAULT_REGION", "value": region}, map[string]string{"name": "AWS_STS_REGIONAL_ENDPOINTS", "value": "regional"}, map[string]string{"name": "AWS_EC2_METADATA_DISABLED", "value": "true"}}
}

func projectedIdentity() map[string]any {
	return map[string]any{"defaultMode": 288, "sources": []any{map[string]any{"serviceAccountToken": map[string]any{"audience": "sts.amazonaws.com", "expirationSeconds": 3600, "path": "aws/token"}}}}
}

func (driver *Driver) configureSource(ctx context.Context, source *unstructured.Unstructured, item cloud.Intent, objectName, region string) error {
	owner := source.GetAnnotations()["keldock.io/backup-binding"]
	if owner != "" && owner != cloud.BackupKey(item.Scope, item.ID) {
		return fmt.Errorf("another backup writer owns source fields")
	}
	if owner == "" {
		plugins, _, _ := unstructured.NestedSlice(source.Object, "spec", "plugins")
		projected, _, _ := unstructured.NestedMap(source.Object, "spec", "projectedVolumeTemplate")
		env, _, _ := unstructured.NestedSlice(source.Object, "spec", "env")
		if len(plugins) > 0 || len(projected) > 0 || len(env) > 0 {
			return fmt.Errorf("existing backup/identity fields require explicit release")
		}
	}
	binding := item.Backup
	spec := map[string]any{"env": awsEnvironment(binding.WriterRoleARN, region), "projectedVolumeTemplate": projectedIdentity(), "plugins": []any{map[string]any{"name": "barman-cloud.cloudnative-pg.io", "isWALArchiver": true, "parameters": map[string]string{"barmanObjectName": objectName}}}}
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster", "metadata": map[string]any{"name": source.GetName(), "namespace": source.GetNamespace(), "uid": string(source.GetUID()), "annotations": map[string]string{"keldock.io/backup-binding": cloud.BackupKey(item.Scope, item.ID)}}, "spec": spec}}
	_, err := driver.Dynamic.Resource(cnpgClusters).Namespace(source.GetNamespace()).Apply(ctx, source.GetName(), object, metav1.ApplyOptions{FieldManager: "keldock-cloud-backup"})
	return err
}

func (driver *Driver) disconnectBackup(ctx context.Context, conn cloud.Connection, item cloud.Intent, source *unstructured.Unstructured) cloud.Evidence {
	if owner := source.GetAnnotations()["keldock.io/backup-binding"]; owner != "" && owner != cloud.BackupKey(item.Scope, item.ID) {
		return evidence("failed", "BackupSourceOwnershipConflict", conn.Generation)
	}
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster", "metadata": map[string]any{"name": source.GetName(), "namespace": source.GetNamespace(), "uid": string(source.GetUID())}, "spec": map[string]any{}}}
	if _, err := driver.Dynamic.Resource(cnpgClusters).Namespace(source.GetNamespace()).Apply(ctx, source.GetName(), object, metav1.ApplyOptions{FieldManager: "keldock-cloud-backup"}); err != nil {
		return unavailable(err, conn.Generation)
	}
	name := "schedule-" + cloud.BackupKey(item.Scope, item.ID)
	client := driver.Dynamic.Resource(schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "scheduledbackups"}).Namespace(source.GetNamespace())
	if _, err := client.Patch(ctx, name, types.MergePatchType, []byte(`{"spec":{"suspend":true}}`), metav1.PatchOptions{FieldManager: "keldock-cloud-backup"}); err != nil {
		return unavailable(err, conn.Generation)
	}
	state := "retained"
	if item.Action == "disconnect" || item.Action == "disconnected" {
		state = "disconnected"
	}
	return evidence(state, "ArchiveDataRetainedAndWriterDisconnected", conn.Generation)
}

func (driver *Driver) restore(ctx context.Context, policy cloud.Policy, conn cloud.Connection, item cloud.Intent, resource cloud.Resource) (bool, error) {
	binding := item.Backup
	target := binding.Database
	namespace, err := driver.Kubernetes.CoreV1().Namespaces().Get(ctx, target.RestoreNamespace, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if namespace.Labels["keldock.io/cloud-sandbox"] != policy.OrganizationID {
		return false, fmt.Errorf("restore namespace lacks operator delegation")
	}
	prefix := ""
	for _, archive := range binding.Archives {
		if archive.Generation == binding.RestoreGeneration {
			prefix = archive.Prefix
		}
	}
	if prefix == "" {
		return false, fmt.Errorf("restore generation is not retained")
	}
	objectName := fmt.Sprintf("restore-%s-g%d", cloud.BackupKey(item.Scope, item.ID), binding.RestoreGeneration)
	if err := driver.applyBackupObject(ctx, objectStore(target.RestoreNamespace, objectName, resource.ExternalName, prefix), cloud.BackupKey(item.Scope, item.ID)); err != nil {
		return false, err
	}
	cluster := map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster", "metadata": map[string]any{"name": binding.RestoreName, "namespace": target.RestoreNamespace}, "spec": map[string]any{
		"instances": 1, "imageName": target.Image, "storage": map[string]string{"size": target.StorageSize, "storageClass": target.StorageClass}, "env": awsEnvironment(binding.RestoreRoleARN, conn.Region), "projectedVolumeTemplate": projectedIdentity(),
		"bootstrap": map[string]any{"recovery": map[string]string{"source": "archive"}}, "externalClusters": []any{map[string]any{"name": "archive", "plugin": map[string]any{"name": "barman-cloud.cloudnative-pg.io", "parameters": map[string]string{"barmanObjectName": objectName, "serverName": target.Name}}}},
	}}
	if err := driver.applyBackupObject(ctx, cluster, cloud.BackupKey(item.Scope, item.ID)); err != nil {
		return false, err
	}
	actual, err := driver.Dynamic.Resource(cnpgClusters).Namespace(target.RestoreNamespace).Get(ctx, binding.RestoreName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return conditionTrue(actual, "Ready"), nil
}
