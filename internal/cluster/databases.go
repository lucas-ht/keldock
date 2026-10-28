package cluster

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (r *Registrar) projectDatabases(ctx context.Context, desired v1.ProjectedDeployment) error {
	s := desired.Selection
	active := map[string]bool{}
	for alias, b := range s.Databases {
		id := database.ID(s.EnvironmentID, alias)
		if b.ResourceID != id || b.Namespace != "kd-"+strings.TrimPrefix(id, "db-") || b.Cluster != "database" || b.Secret != "keldock-db-"+alias {
			return fmt.Errorf("invalid database binding placement")
		}
		cluster, err := r.client.Resource(schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}).Namespace(b.Namespace).Get(ctx, b.Cluster, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("database allocation is unavailable")
		}
		namespace, err := r.resource("Namespace", "").Get(ctx, b.Namespace, metav1.GetOptions{})
		if err != nil || namespace.GetAnnotations()["keldock.io/source-uid"] == "" || namespace.GetAnnotations()["keldock.io/source-uid"] != string(cluster.GetUID()) {
			return fmt.Errorf("database identity requires operator recovery")
		}
		for key, value := range map[string]string{"app.kubernetes.io/managed-by": "keldock", "keldock.io/organization": desired.Operation.OrganizationID, "keldock.io/project": desired.Operation.ProjectID, "keldock.io/environment": s.EnvironmentID, "keldock.io/allocation": b.ResourceID} {
			if cluster.GetLabels()[key] != value {
				return fmt.Errorf("database allocation owner mismatch")
			}
		}
		conditions, _, _ := unstructured.NestedSlice(cluster.Object, "status", "conditions")
		ready := false
		for _, raw := range conditions {
			condition, ok := raw.(map[string]any)
			if ok && condition["type"] == "Ready" && condition["status"] == "True" {
				ready = true
			}
		}
		if !ready {
			return fmt.Errorf("waiting for PostgreSQL readiness")
		}
		credentials, err := r.resource("Secret", b.Namespace).Get(ctx, "database-app", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("database application credential unavailable")
		}
		ca, err := r.resource("Secret", b.Namespace).Get(ctx, "database-ca", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("database TLS authority unavailable")
		}
		password, _, _ := unstructured.NestedString(credentials.Object, "data", "password")
		username, _, _ := unstructured.NestedString(credentials.Object, "data", "username")
		root, _, _ := unstructured.NestedString(ca.Object, "data", "ca.crt")
		if password == "" || username == "" || root == "" {
			return fmt.Errorf("database credential is incomplete")
		}
		meta := metadata(b.Secret, s)
		meta.Labels["keldock.io/database-binding"] = b.ResourceID
		object := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": meta, "type": "Opaque", "data": map[string]string{"username": username, "password": password, "ca.crt": root}}
		if err := r.apply(ctx, object, s.EnvironmentID, s.Generation); err != nil {
			return err
		}
		active[b.Secret] = true
	}
	secrets, err := r.resource("Secret", s.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "keldock.io/database-binding"})
	if err != nil {
		return err
	}
	for _, secret := range secrets.Items {
		if !active[secret.GetName()] {
			if _, err := r.remove(ctx, "Secret", s.Namespace, secret.GetName(), s.EnvironmentID, s.Generation); err != nil {
				return err
			}
		}
	}
	return nil
}
