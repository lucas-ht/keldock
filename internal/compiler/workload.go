package compiler

import (
	"crypto/sha256"
	"fmt"
	"slices"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func labels(name string, selection Selection) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "keldock", "keldock.io/environment": selection.EnvironmentID, "keldock.io/component": name}
}

func pod(name string, component v1.Component, image string, selection Selection) corev1.PodTemplateSpec {
	container := corev1.Container{Name: name, Image: image, Command: component.Command, Args: component.Args,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To(int64(65532)),
			ReadOnlyRootFilesystem: ptr.To(true), AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")}},
		VolumeMounts: []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}}}
	if component.Port > 0 {
		container.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: component.Port}}
	}
	keys := make([]string, 0, len(component.Env))
	for key := range component.Env {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: component.Env[key]})
	}
	if component.Probe != nil {
		handler := corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: component.Probe.Command}}
		if component.Probe.Path != "" {
			handler = corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: component.Probe.Path, Port: intstr.FromInt32(component.Port)}}
		}
		container.ReadinessProbe = &corev1.Probe{ProbeHandler: handler, PeriodSeconds: 2, TimeoutSeconds: 1, FailureThreshold: 3}
		container.LivenessProbe = &corev1.Probe{ProbeHandler: handler, PeriodSeconds: 10, TimeoutSeconds: 1, FailureThreshold: 6}
	}
	return corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels(name, selection)}, Spec: corev1.PodSpec{
		ServiceAccountName: "workload", AutomountServiceAccountToken: ptr.To(false), TerminationGracePeriodSeconds: ptr.To(int64(5)),
		SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To(int64(65532)), FSGroup: ptr.To(int64(65532)),
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
		Containers: []corev1.Container{container}, Volumes: []corev1.Volume{{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptr.To(resource.MustParse("16Mi"))}}}}}}
}

func workload(name string, component v1.Component, image string, selection Selection) []any {
	metadata := metav1.ObjectMeta{Name: name, Namespace: selection.Namespace, Labels: labels(name, selection), Annotations: map[string]string{"keldock.io/deployment": selection.DeploymentID}}
	template := pod(name, component, image, selection)
	if binding, ok := selection.Cloud[name]; ok {
		bindCloudPod(&template, binding)
	}
	var result []any
	switch component.Kind {
	case "http", "worker":
		result = append(result, &appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}, ObjectMeta: metadata,
			Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(1)), Selector: &metav1.LabelSelector{MatchLabels: labels(name, selection)}, Template: template}})
	case "job":
		metadata.Name = JobName(name, selection.DeploymentID)
		template.Spec.RestartPolicy = corev1.RestartPolicyNever
		result = append(result, &batchv1.Job{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"}, ObjectMeta: metadata,
			Spec: batchv1.JobSpec{BackoffLimit: ptr.To(int32(0)), ActiveDeadlineSeconds: ptr.To(int64(60)), Template: template}})
	case "cronjob":
		template.Spec.RestartPolicy = corev1.RestartPolicyNever
		result = append(result, &batchv1.CronJob{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"}, ObjectMeta: metadata,
			Spec: batchv1.CronJobSpec{Schedule: component.Schedule, ConcurrencyPolicy: batchv1.ForbidConcurrent, SuccessfulJobsHistoryLimit: ptr.To(int32(1)), FailedJobsHistoryLimit: ptr.To(int32(1)),
				JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{BackoffLimit: ptr.To(int32(0)), ActiveDeadlineSeconds: ptr.To(int64(60)), Template: template}}}})
	}
	if component.Port > 0 {
		result = append(result, &corev1.Service{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}, ObjectMeta: metadata,
			Spec: corev1.ServiceSpec{Selector: labels(name, selection), Ports: []corev1.ServicePort{{Name: "http", Port: component.Port, TargetPort: intstr.FromString("http")}}}})
	}
	if component.Route && selection.Hostname != "" {
		result = append(result, &networkingv1.Ingress{TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}, ObjectMeta: metadata,
			Spec: networkingv1.IngressSpec{IngressClassName: ptr.To("traefik"), Rules: []networkingv1.IngressRule{{Host: selection.Hostname, IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: ptr.To(networkingv1.PathTypePrefix), Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{Name: name, Port: networkingv1.ServiceBackendPort{Name: "http"}}}}}}}}}}})
	}
	for _, route := range selection.Domains {
		if route.Component != name {
			continue
		}
		meta := metadata
		meta.Name = DomainIngressName(route.ID)
		result = append(result, certificate(route, selection), &networkingv1.Ingress{TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}, ObjectMeta: meta, Spec: networkingv1.IngressSpec{IngressClassName: ptr.To("traefik"), TLS: []networkingv1.IngressTLS{{Hosts: []string{route.Hostname}, SecretName: CertificateName(route.ID)}}, Rules: []networkingv1.IngressRule{{Host: route.Hostname, IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: ptr.To(networkingv1.PathTypePrefix), Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: name, Port: networkingv1.ServiceBackendPort{Name: "http"}}}}}}}}}}})
	}
	return result
}

// JobName is stable per selected deployment and stays within Kubernetes' DNS label limit.
func JobName(component, deployment string) string {
	return component + "-" + fmt.Sprintf("%x", sha256.Sum256([]byte(deployment)))[:16]
}
