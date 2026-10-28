package compiler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func validateCloud(app v1.Application, selection Selection) error {
	if len(selection.Cloud) > 16 || len(selection.Domains) > 16 {
		return fmt.Errorf("cloud placement exceeds bounded consumers")
	}
	for name, binding := range selection.Cloud {
		if _, ok := app.Components[name]; !ok || !v1.ValidName(binding.ServiceAccount) || !strings.HasPrefix(binding.RoleARN, "arn:aws:iam::") || !strings.Contains(binding.RoleARN, ":role/keldock/") || len(binding.RoleARN) > 512 || binding.Region == "" || len(binding.Stores) == 0 || len(binding.Stores) > 16 || len(binding.EgressCIDRs) == 0 || len(binding.EgressCIDRs) > 128 {
			return fmt.Errorf("invalid trusted cloud placement")
		}
		for _, cidr := range binding.EgressCIDRs {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return fmt.Errorf("invalid cloud egress")
			}
		}
	}
	for _, route := range selection.Domains {
		component, ok := app.Components[route.Component]
		if !ok || !component.Route || !v1.ValidName(route.ID) || !v1.ValidName(route.IssuerRef) || strings.ContainsAny(route.Hostname, "/:@ *\n") || route.Hostname == "" {
			return fmt.Errorf("invalid trusted domain placement")
		}
	}
	return nil
}

func bindCloudPod(template *corev1.PodTemplateSpec, binding v1.WorkloadCloud) {
	template.Spec.ServiceAccountName = binding.ServiceAccount
	stores, _ := json.Marshal(binding.Stores)
	values := map[string]string{"AWS_ROLE_ARN": binding.RoleARN, "AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/keldock/aws/token", "AWS_REGION": binding.Region, "AWS_DEFAULT_REGION": binding.Region, "AWS_STS_REGIONAL_ENDPOINTS": "regional", "AWS_EC2_METADATA_DISABLED": "true", "KELDOCK_OBJECT_STORES": string(stores)}
	container := &template.Spec.Containers[0]
	container.Env = slices.DeleteFunc(container.Env, func(value corev1.EnvVar) bool {
		return strings.HasPrefix(value.Name, "AWS_") || value.Name == "KELDOCK_OBJECT_STORES"
	})
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: values[key]})
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "aws-identity", MountPath: "/var/run/keldock/aws", ReadOnly: true})
	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{Name: "aws-identity", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{DefaultMode: ptr.To(int32(0o440)), Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Audience: "sts.amazonaws.com", ExpirationSeconds: ptr.To(int64(3600)), Path: "token"}}}}}})
}

func certificate(route v1.DomainRoute, selection Selection) map[string]any {
	name := CertificateName(route.ID)
	return map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": metav1.ObjectMeta{Name: name, Namespace: selection.Namespace, Labels: labels(route.Component, selection)}, "spec": map[string]any{"secretName": name, "dnsNames": []string{route.Hostname}, "issuerRef": map[string]string{"name": route.IssuerRef, "kind": "ClusterIssuer"}}}
}

// CertificateName is bounded even for a maximum-length intent identifier.
func CertificateName(id string) string {
	return "tls-" + fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:24]
}

// DomainIngressName is independent from the default application ingress.
func DomainIngressName(id string) string {
	return "route-" + fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:24]
}
