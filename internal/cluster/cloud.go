package cluster

import (
	"context"

	"github.com/lucas-ht/keldock/internal/compiler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Keep certificate cleanup authority until namespace teardown after the first grant.
// Removing it before Flux prunes a disconnected route would strand that certificate.
func (registrar *Registrar) certificateAuthority(ctx context.Context, selection compiler.Selection) (bool, error) {
	if len(selection.Domains) > 0 {
		return true, nil
	}
	role, err := registrar.resource("Role", selection.Namespace).Get(ctx, "keldock-reconciler", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rules, _, err := unstructured.NestedSlice(role.Object, "rules")
	if err != nil {
		return false, err
	}
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
		for _, group := range groups {
			if group == "cert-manager.io" {
				return true, nil
			}
		}
	}
	return false, nil
}
