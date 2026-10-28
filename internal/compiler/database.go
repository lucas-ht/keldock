package compiler

import (
	"fmt"
	"slices"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func validateDatabases(app v1.Application, selection Selection) error {
	for alias, binding := range selection.Databases {
		request, ok := app.Resources[alias]
		id := database.ID(selection.EnvironmentID, alias)
		if !ok || request.Kind != "postgresql" || binding.ResourceID != id || binding.Namespace != "kd-"+strings.TrimPrefix(id, "db-") || binding.Cluster != "database" || binding.Secret != "keldock-db-"+alias || binding.Host != "database-rw."+binding.Namespace+".svc.cluster.local" {
			return fmt.Errorf("database placement outside trusted allocation")
		}
		seen := map[string]bool{}
		for _, name := range binding.Components {
			component, ok := app.Components[name]
			if !ok || !slices.Contains(component.Resources, alias) || seen[name] {
				return fmt.Errorf("database consumer outside declared component bindings")
			}
			seen[name] = true
		}
	}
	return nil
}

func databaseFor(name string, selection Selection) (string, v1.DatabaseBinding, bool) {
	for alias, binding := range selection.Databases {
		if slices.Contains(binding.Components, name) {
			return alias, binding, true
		}
	}
	return "", v1.DatabaseBinding{}, false
}

func bindDatabasePod(template *corev1.PodTemplateSpec, name string, selection Selection) {
	alias, binding, ok := databaseFor(name, selection)
	if !ok {
		return
	}
	container := &template.Spec.Containers[0]
	container.Env = slices.DeleteFunc(container.Env, func(env corev1.EnvVar) bool { return strings.HasPrefix(env.Name, "PG") || env.Name == "DATABASE_URL" })
	for key, value := range map[string]string{"PGHOST": binding.Host, "PGPORT": "5432", "PGDATABASE": "app", "PGSSLMODE": "verify-full", "PGSSLROOTCERT": "/var/run/keldock/database/" + alias + "/ca.crt"} {
		container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: value})
	}
	// Keep deterministic bundle bytes despite Go map iteration order.
	slices.SortFunc(container.Env, func(a, b corev1.EnvVar) int { return strings.Compare(a.Name, b.Name) })
	for _, entry := range []struct{ Name, Key string }{{"PGUSER", "username"}, {"PGPASSWORD", "password"}} {
		container.Env = append(container.Env, corev1.EnvVar{Name: entry.Name, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: binding.Secret}, Key: entry.Key}}})
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "database", MountPath: "/var/run/keldock/database/" + alias, ReadOnly: true})
	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{Name: "database", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: binding.Secret, Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}, DefaultMode: ptr.To(int32(0444))}}})
}

func databaseEgress(policy *networkingv1.NetworkPolicy, name string, selection Selection) {
	_, binding, ok := databaseFor(name, selection)
	if !ok {
		return
	}
	policy.Spec.Egress = append(policy.Spec.Egress, networkingv1.NetworkPolicyEgressRule{To: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": binding.Namespace}}, PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"cnpg.io/cluster": binding.Cluster}}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(5432))}}})
}
