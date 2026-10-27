package cluster

import (
	"context"
	"fmt"
	"strconv"

	"github.com/lucas-ht/keldock/internal/compiler"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func metadata(name string, selection compiler.Selection) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: selection.Namespace,
		Labels:      map[string]string{"app.kubernetes.io/managed-by": "keldock", "keldock.io/environment": selection.EnvironmentID, "keldock.io/deployment": selection.DeploymentID},
		Annotations: map[string]string{"keldock.io/generation": strconv.FormatInt(selection.Generation, 10)}}
}

func (registrar *Registrar) register(ctx context.Context, selection compiler.Selection, privateImages bool) error {
	meta := metadata(selection.Namespace, selection)
	meta.Namespace = ""
	meta.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
	meta.Labels["pod-security.kubernetes.io/enforce-version"] = "v1.35"
	namespace := &corev1.Namespace{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"}, ObjectMeta: meta}
	if err := registrar.apply(ctx, namespace, selection.EnvironmentID, selection.Generation); err != nil {
		return fmt.Errorf("register namespace: %w", err)
	}
	pullSecrets := []map[string]string{}
	if privateImages {
		pullSecrets = append(pullSecrets, map[string]string{"name": "keldock-image-pull"})
	}
	workload := map[string]any{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": metadata("workload", selection), "automountServiceAccountToken": false, "imagePullSecrets": pullSecrets}
	if err := registrar.apply(ctx, workload, selection.EnvironmentID, selection.Generation); err != nil {
		return fmt.Errorf("register workload identity: %w", err)
	}
	objects := []any{
		&corev1.ServiceAccount{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"}, ObjectMeta: metadata("reconciler", selection), AutomountServiceAccountToken: ptr.To(false)},
		&rbacv1.Role{TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"}, ObjectMeta: metadata("keldock-reconciler", selection), Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"batch"}, Resources: []string{"jobs", "cronjobs"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{APIGroups: []string{""}, Resources: []string{"pods", "events"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"networking.k8s.io"}, Resources: []string{"ingresses", "networkpolicies"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		}},
		&rbacv1.RoleBinding{TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"}, ObjectMeta: metadata("keldock-reconciler", selection),
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "reconciler", Namespace: selection.Namespace}}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "keldock-reconciler"}},
		&networkingv1.NetworkPolicy{TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "NetworkPolicy"}, ObjectMeta: metadata("keldock-default-deny", selection),
			Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}}},
		&corev1.ResourceQuota{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ResourceQuota"}, ObjectMeta: metadata("keldock-limits", selection), Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
			corev1.ResourceRequestsCPU: resource.MustParse("2"), corev1.ResourceRequestsMemory: resource.MustParse("2Gi"), corev1.ResourceLimitsCPU: resource.MustParse("8"), corev1.ResourceLimitsMemory: resource.MustParse("4Gi"), corev1.ResourcePods: resource.MustParse("64")}}},
	}
	for _, object := range objects {
		if err := registrar.apply(ctx, object, selection.EnvironmentID, selection.Generation); err != nil {
			return fmt.Errorf("register environment guardrails: %w", err)
		}
	}
	return nil
}
