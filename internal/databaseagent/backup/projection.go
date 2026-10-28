package backup

import (
	"context"
	"fmt"

	"github.com/lucas-ht/keldock/internal/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (d *Service) backupApply(ctx context.Context, a database.Allocation, value map[string]any) (*unstructured.Unstructured, error) {
	return d.apply(ctx, a, value, "keldock-database-backup", "keldock.io/backup-generation")
}

func archiveObject(a database.Allocation, destination database.Destination, namespace, secret string) map[string]any {
	configuration := map[string]any{"destinationPath": "s3://" + destination.Bucket + "/" + database.ArchivePrefix(a.Scope, a.ID), "endpointURL": destination.Endpoint, "s3Credentials": map[string]any{"accessKeyId": map[string]string{"name": secret, "key": "ACCESS_KEY_ID"}, "secretAccessKey": map[string]string{"name": secret, "key": "ACCESS_SECRET_KEY"}}, "wal": map[string]string{"compression": "gzip"}, "data": map[string]string{"compression": "gzip"}}
	if destination.CA != nil {
		configuration["endpointCA"] = map[string]string{"name": "archive-ca", "key": "ca.crt"}
	}
	return map[string]any{"apiVersion": "barmancloud.cnpg.io/v1", "kind": "ObjectStore", "metadata": map[string]any{"name": "keldock-archive", "namespace": namespace}, "spec": map[string]any{"configuration": configuration, "instanceSidecarConfiguration": map[string]any{"env": []any{map[string]string{"name": "AWS_DEFAULT_REGION", "value": destination.Region}, map[string]string{"name": "AWS_REGION", "value": destination.Region}, map[string]string{"name": "AWS_EC2_METADATA_DISABLED", "value": "true"}}, "resources": map[string]any{"requests": map[string]string{"cpu": "25m", "memory": "64Mi"}, "limits": map[string]string{"cpu": "500m", "memory": "256Mi"}}}}}
}

func (d *Service) configureBackupSource(ctx context.Context, a database.Allocation, source *unstructured.Unstructured, paused bool) error {
	owner := "postgresql/" + a.OrganizationID + "/" + a.ProjectID + "/" + a.ID
	if current := source.GetAnnotations()["keldock.io/backup-binding"]; current != "" && current != owner {
		return fmt.Errorf("another backup controller owns this source")
	}
	if source.GetAnnotations()["keldock.io/backup-binding"] == "" {
		plugins, _, _ := unstructured.NestedSlice(source.Object, "spec", "plugins")
		if len(plugins) > 0 {
			return fmt.Errorf("existing backup integration requires explicit release")
		}
	}
	plugins := []any{}
	if !paused {
		plugins = append(plugins, map[string]any{"name": "barman-cloud.cloudnative-pg.io", "isWALArchiver": true, "parameters": map[string]string{"barmanObjectName": "keldock-archive"}})
	}
	_, err := d.backupApply(ctx, a, map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster", "metadata": map[string]any{"name": "database", "namespace": a.Namespace, "uid": string(source.GetUID()), "annotations": map[string]string{"keldock.io/backup-binding": owner}}, "spec": map[string]any{"plugins": plugins}})
	return err
}

func (d *Service) restoreBackup(ctx context.Context, a database.Allocation, destination database.Destination, reader map[string]string) (*unstructured.Unstructured, error) {
	namespace := database.RestoreNamespace(a.ID, a.Backup.RestoreAttempt)
	if _, err := d.backupApply(ctx, a, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": namespace, "labels": map[string]string{"keldock.io/retained": "postgresql-restore", "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.35"}}}); err != nil {
		return nil, err
	}
	if err := d.projectBackupSecret(ctx, a, namespace, "archive-reader", reader); err != nil {
		return nil, err
	}
	if destination.CA != nil {
		ca, err := d.secret(ctx, *destination.CA)
		if err != nil {
			return nil, err
		}
		if err := d.projectBackupSecret(ctx, a, namespace, "archive-ca", map[string]string{"ca.crt": ca["ca.crt"]}); err != nil {
			return nil, err
		}
	}
	if _, err := d.backupApply(ctx, a, archiveObject(a, destination, namespace, "archive-reader")); err != nil {
		return nil, err
	}
	restore := d.cluster(a)
	restore["metadata"] = map[string]any{"name": "restore", "namespace": namespace}
	spec := restore["spec"].(map[string]any)
	spec["bootstrap"] = map[string]any{"recovery": map[string]string{"source": "archive"}}
	delete(spec, "managed")
	spec["externalClusters"] = []any{map[string]any{"name": "archive", "plugin": map[string]any{"name": "barman-cloud.cloudnative-pg.io", "parameters": map[string]string{"barmanObjectName": "keldock-archive", "serverName": "database"}}}}
	// No application ingress/binding is created for an isolated restored cluster.
	policy := d.network(a)
	policy["metadata"] = map[string]any{"name": "restore", "namespace": namespace}
	// Preserve the operator's instance-manager health port, with no application ingress.
	policy["spec"].(map[string]any)["ingress"] = policy["spec"].(map[string]any)["ingress"].([]any)[:1]
	if _, err := d.backupApply(ctx, a, policy); err != nil {
		return nil, err
	}
	if _, err := d.backupApply(ctx, a, restore); err != nil {
		return nil, err
	}
	return d.Client.Resource(resources["Cluster"]).Namespace(namespace).Get(ctx, "restore", metav1.GetOptions{})
}
