package cluster

import (
	"context"

	"github.com/lucas-ht/keldock/internal/compiler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Suspend stops future Flux steps while retaining completed workload effects and data.
func (registrar *Registrar) Suspend(ctx context.Context, selection compiler.Selection) error {
	if err := registrar.checkpoint(ctx, JournalEntry{EnvironmentID: selection.EnvironmentID, Generation: selection.Generation, Action: "suspend", DeploymentID: selection.DeploymentID}); err != nil {
		return err
	}
	namespace, err := registrar.resource("Namespace", "").Get(ctx, selection.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := owned(namespace, selection.EnvironmentID, selection.Generation); err != nil {
		return err
	}
	for _, kind := range []string{"Kustomization", "OCIRepository"} {
		objects, err := registrar.resource(kind, selection.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "keldock.io/environment=" + selection.EnvironmentID})
		if err != nil {
			return err
		}
		for _, object := range objects.Items {
			if err := owned(&object, selection.EnvironmentID, selection.Generation); err != nil {
				return err
			}
			if object.GetDeletionTimestamp() != nil {
				continue
			}
			suspended, _, _ := unstructured.NestedBool(object.Object, "spec", "suspend")
			if suspended {
				continue
			}
			if err := unstructured.SetNestedField(object.Object, true, "spec", "suspend"); err != nil {
				return err
			}
			if _, err := registrar.resource(kind, selection.Namespace).Update(ctx, &object, metav1.UpdateOptions{FieldManager: "keldock-registrar"}); err != nil {
				return err
			}
		}
	}
	return nil
}
