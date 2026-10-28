package cloudagent

import (
	"context"
	"slices"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Wait for the actual rolled-out identity and pinned plugin, not an older Cluster Ready condition.
func (driver *Driver) backupPodReady(ctx context.Context, target cloud.DatabaseTarget, role string) (bool, error) {
	pins, err := crossplane.Locked()
	if err != nil {
		return false, err
	}
	image := pins.Images["ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar:v0.15.1"]
	pods, err := driver.Kubernetes.CoreV1().Pods(target.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "cnpg.io/cluster=" + target.Name})
	if err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil || pod.Spec.ServiceAccountName != target.Name {
			continue
		}
		identity, plugin, ready := false, false, false
		containers := append(slices.Clone(pod.Spec.Containers), pod.Spec.InitContainers...)
		for _, container := range containers {
			if container.Image == image {
				plugin = true
			}
			if container.Name == "postgres" {
				for _, value := range container.Env {
					if value.Name == "AWS_ROLE_ARN" && value.Value == role {
						identity = true
					}
				}
			}
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if identity && plugin && ready {
			return true, nil
		}
	}
	return false, nil
}

// CNPG updates plugin RBAC asynchronously when the archive ObjectStore changes.
// Do not consume a one-shot base-backup attempt before that exact grant exists.
func (driver *Driver) backupAuthorityReady(ctx context.Context, policy cloud.Policy, conn cloud.Connection, binding cloud.BackupBinding, bucket, object string) (bool, error) {
	target := binding.Database
	for _, permission := range []struct{ verb, subresource string }{{"get", ""}, {"update", "status"}} {
		review, err := driver.Kubernetes.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{User: "system:serviceaccount:" + target.Namespace + ":" + target.Name, ResourceAttributes: &authorizationv1.ResourceAttributes{Namespace: target.Namespace, Group: "barmancloud.cnpg.io", Resource: "objectstores", Name: object, Verb: permission.verb, Subresource: permission.subresource}}}, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		if !review.Status.Allowed {
			return false, nil
		}
	}
	identity := cloud.IdentityProfile{Account: conn.Account, Issuer: policy.Identities[conn.IdentityRef].Issuer, Namespace: target.Namespace, Roles: map[string]string{"s3": binding.WriterRoleARN}, ServiceAccounts: map[string]string{"s3": target.Name}}
	config, err := driver.Probe.Config(ctx, conn, identity, "s3")
	if err != nil {
		return false, err
	}
	return true, awsprobe.ArchiveAccess(ctx, config, conn, bucket, binding.ArchivePrefix)
}
