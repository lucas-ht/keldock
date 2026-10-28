package cloudagent

import (
	"context"
	"slices"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
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
