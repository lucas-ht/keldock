package databaseagent

import (
	"strings"

	"github.com/lucas-ht/keldock/internal/database"
)

func network(a database.Allocation) map[string]any {
	ingress := []any{map[string]any{"from": []any{map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]string{"kubernetes.io/metadata.name": "cnpg-system"}}}}, "ports": []any{map[string]any{"protocol": "TCP", "port": 8000}}}, map[string]any{"from": []any{map[string]any{"podSelector": map[string]any{"matchLabels": map[string]string{"cnpg.io/cluster": "database"}}}}, "ports": []any{map[string]any{"protocol": "TCP", "port": 5432}}}}
	if a.Action == "reconcile" {
		for _, component := range a.Components {
			ingress = append(ingress, map[string]any{"from": []any{map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]string{"kubernetes.io/metadata.name": "ke-" + strings.TrimPrefix(a.EnvironmentID, "env-")}}, "podSelector": map[string]any{"matchLabels": map[string]string{"keldock.io/environment": a.EnvironmentID, "keldock.io/component": component}}}}, "ports": []any{map[string]any{"protocol": "TCP", "port": 5432}}})
		}
	}
	return map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]any{"name": "database", "namespace": a.Namespace}, "spec": map[string]any{"podSelector": map[string]any{}, "policyTypes": []string{"Ingress", "Egress"}, "ingress": ingress, "egress": []any{
		map[string]any{"to": []any{map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]string{"kubernetes.io/metadata.name": "kube-system"}}, "podSelector": map[string]any{"matchLabels": map[string]string{"k8s-app": "kube-dns"}}}}, "ports": []any{map[string]any{"protocol": "UDP", "port": 53}, map[string]any{"protocol": "TCP", "port": 53}}},
		map[string]any{"to": []any{map[string]any{"podSelector": map[string]any{}}}, "ports": []any{map[string]any{"protocol": "TCP", "port": 5432}, map[string]any{"protocol": "TCP", "port": 8000}}},
		// CNPG's instance manager and the native backup sidecar need the API server.
		map[string]any{"ports": []any{map[string]any{"protocol": "TCP", "port": 443}, map[string]any{"protocol": "TCP", "port": 6443}}},
	}}}
}
