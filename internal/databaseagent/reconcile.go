package databaseagent

import (
	"context"
	"fmt"

	"github.com/lucas-ht/keldock/internal/database"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Reconcile creates retained substrate, updates authorized access and reports actual CNPG health.
func (d *Driver) Reconcile(ctx context.Context, p database.Policy, a database.Allocation) database.Evidence {
	e := database.Evidence{Scope: a.Scope, ID: a.ID, Generation: a.Generation, State: "pending", Reason: "DatabaseStarting"}
	if p.Validate() != nil || !a.ValidProjection(p) {
		e.State = "failed"
		e.Reason = "OperatorDelegationDenied"
		return e
	}
	if existing, err := d.Client.Resource(resources["Namespace"]).Get(ctx, a.Namespace, metav1.GetOptions{}); err == nil {
		remembered := existing.GetAnnotations()["keldock.io/source-uid"]
		if a.SourceUID != "" && remembered != "" && a.SourceUID != remembered {
			e.State = "failed"
			e.Reason = "DatabaseIdentityChanged"
			return e
		}
		if a.SourceUID == "" {
			a.SourceUID = remembered
		}
	} else if !apierrors.IsNotFound(err) {
		e.State = "failed"
		e.Reason = "DatabaseInventoryUnavailable"
		return e
	}
	if a.SourceUID != "" {
		current, err := d.Client.Resource(resources["Cluster"]).Namespace(a.Namespace).Get(ctx, "database", metav1.GetOptions{})
		if err != nil || string(current.GetUID()) != a.SourceUID {
			e.State = "failed"
			e.Reason = "DatabaseIdentityChanged_RestoreRequired"
			return e
		}
	}
	ns := databaseNamespace(a)
	for _, object := range []map[string]any{ns, network(a), cluster(a)} {
		actual, err := d.apply(ctx, a, object)
		if err != nil {
			e.State = "failed"
			e.Reason = "DatabaseReconciliationBlocked"
			return e
		}
		if actual.GetKind() != "Cluster" {
			continue
		}
		e.UID = string(actual.GetUID())
		if a.SourceUID == "" && e.UID != "" {
			a.SourceUID = e.UID
			if _, err := d.apply(ctx, a, databaseNamespace(a)); err != nil {
				e.State = "failed"
				e.Reason = "DatabaseIdentityCheckpointFailed"
				return e
			}
		}
		conditions, _, _ := unstructured.NestedSlice(actual.Object, "status", "conditions")
		for _, raw := range conditions {
			condition, ok := raw.(map[string]any)
			if ok && condition["type"] == "Ready" && condition["status"] == "True" {
				e.State = "ready"
				e.Reason = "PostgreSQLReady"
			}
		}
	}
	if a.Action == "retain" {
		// Retention revokes new logins and ingress; existing sessions end as consumers roll.
		e.State = "retained"
		e.Reason = "DataRetainedConsumersDisconnected"
	}
	if a.Backup != nil {
		e.Backup = d.backup(ctx, p, a)
	}
	return e
}

// Inspect requires the exact tested operator rather than adopting a similar controller.
func (d *Driver) Inspect(ctx context.Context) error {
	operator, err := d.Client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("cnpg-system").Get(ctx, "cnpg-controller-manager", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("tested CNPG operator is unavailable")
	}
	containers, _, _ := unstructured.NestedSlice(operator.Object, "spec", "template", "spec", "containers")
	matched := false
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if ok && container["image"] == "ghcr.io/cloudnative-pg/cloudnative-pg:1.30.1@sha256:923c267ec29636db3bee20f993d0ec4973fa22998e1adad37da79e4d32b5bc07" {
			matched = true
		}
	}
	available, _, _ := unstructured.NestedInt64(operator.Object, "status", "availableReplicas")
	if !matched || available < 1 {
		return fmt.Errorf("CNPG version or availability outside tested profile")
	}
	_, err = d.Client.Resource(resources["Cluster"]).Namespace("default").List(ctx, metav1.ListOptions{Limit: 1})
	return err
}
