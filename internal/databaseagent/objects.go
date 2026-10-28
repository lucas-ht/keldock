package databaseagent

import "github.com/lucas-ht/keldock/internal/database"

func databaseNamespace(a database.Allocation) map[string]any {
	annotations := map[string]string{}
	if a.SourceUID != "" {
		annotations["keldock.io/source-uid"] = a.SourceUID
	}
	return map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": a.Namespace, "annotations": annotations, "labels": map[string]string{"keldock.io/retained": "postgresql", "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.35"}}}
}

func cluster(a database.Allocation) map[string]any {
	return map[string]any{"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster", "metadata": map[string]any{"name": "database", "namespace": a.Namespace}, "spec": map[string]any{
		"instances": 1, "imageName": database.Image, "enableSuperuserAccess": false,
		// Development is a single-instance profile with a bounded maintenance outage.
		// Do not leave a new backup sidecar waiting 30 minutes for old connections.
		"smartShutdownTimeout": 15, "stopDelay": 90,
		"bootstrap":  map[string]any{"initdb": map[string]string{"database": "app", "owner": "app"}},
		"storage":    map[string]string{"size": a.Configuration.StorageSize, "storageClass": a.Configuration.StorageClass},
		"resources":  map[string]any{"requests": map[string]string{"cpu": "100m", "memory": "128Mi"}, "limits": map[string]string{"cpu": "1", "memory": "512Mi"}},
		"postgresql": map[string]any{"parameters": map[string]string{"shared_buffers": "32MB", "max_wal_size": "256MB", "min_wal_size": "80MB", "archive_timeout": "60s"}, "pg_hba": []string{"hostssl app app all scram-sha-256", "host all all all reject"}},
		"managed":    map[string]any{"roles": []any{map[string]any{"name": "app", "login": a.Action == "reconcile", "superuser": false, "createdb": false, "createrole": false, "connectionLimit": 20, "passwordSecret": map[string]string{"name": "database-app"}}}},
	}}
}
