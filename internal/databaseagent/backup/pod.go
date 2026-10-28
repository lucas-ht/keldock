package backup

import (
	"context"

	"github.com/lucas-ht/keldock/internal/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (d *Service) backupPodReady(ctx context.Context, a database.Allocation) bool {
	pods, err := d.Client.Resource(resources["Pod"]).Namespace(a.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "cnpg.io/cluster=database"})
	if err != nil || len(pods.Items) != 1 {
		return false
	}
	pod := pods.Items[0]
	owned := false
	for _, ref := range pod.GetOwnerReferences() {
		if string(ref.UID) == a.SourceUID && ref.Kind == "Cluster" {
			owned = true
		}
	}
	if !owned || !condition(&pod, "Ready") {
		return false
	}
	containers, _, _ := unstructured.NestedSlice(pod.Object, "spec", "initContainers")
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if ok && container["image"] == "ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar:v0.15.1@sha256:2287a793a804db751edd0d07d44cd549e357677e98a800eb12e2cf7c0d53f9b0" && container["restartPolicy"] == "Always" {
			return true
		}
	}
	return false
}
