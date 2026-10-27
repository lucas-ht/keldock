package cluster

import (
	"context"
	"fmt"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Dependency is a read-only inventory result. Discovery never transfers ownership.
type Dependency struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Image     string `json:"image,omitempty"`
	UID       string `json:"uid"`
	Mode      string `json:"mode"`
}

// Inspect verifies the narrow supplied Flux/Traefik profile without applying any object.
// Other installations must add their own compatibility evidence before being supported.
func (registrar *Registrar) Inspect(ctx context.Context) ([]Dependency, error) {
	var result []Dependency
	for _, name := range []string{"source-controller", "kustomize-controller"} {
		object, err := registrar.resource("Deployment", "keldock-system").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("supplied dependency %s unavailable: %w", name, err)
		}
		containers, _, _ := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
		if len(containers) != 1 {
			return nil, fmt.Errorf("unsupported %s container profile", name)
		}
		container, ok := containers[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid %s container profile", name)
		}
		image, _ := container["image"].(string)
		if strings.Split(image, "@")[0] != "ghcr.io/fluxcd/"+name+":v1.10.1" {
			return nil, fmt.Errorf("%s is outside the tested v1.10.1 profile", name)
		}
		observed, _, _ := unstructured.NestedInt64(object.Object, "status", "observedGeneration")
		ready, _, _ := unstructured.NestedInt64(object.Object, "status", "availableReplicas")
		if observed != object.GetGeneration() || ready < 1 {
			return nil, fmt.Errorf("%s is not currently available", name)
		}
		if name == "kustomize-controller" {
			args, _, err := unstructured.NestedStringSlice(container, "args")
			if err != nil {
				return nil, fmt.Errorf("invalid %s isolation arguments", name)
			}
			for _, required := range []string{"--no-cross-namespace-refs=true", "--no-remote-bases=true", "--default-service-account=untrusted"} {
				if !slices.Contains(args, required) {
					return nil, fmt.Errorf("supplied Flux lacks required isolation flag %s", required)
				}
			}
		}
		result = append(result, Dependency{Name: name, Namespace: object.GetNamespace(), Image: image, UID: string(object.GetUID()), Mode: "supplied"})
	}
	crds := registrar.client.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	for _, name := range []string{"ocirepositories.source.toolkit.fluxcd.io", "kustomizations.kustomize.toolkit.fluxcd.io"} {
		crd, err := crds.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("required Flux API %s unavailable: %w", name, err)
		}
		versions, _, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
		served := slices.ContainsFunc(versions, func(item any) bool {
			version, ok := item.(map[string]any)
			return ok && version["name"] == "v1" && version["served"] == true
		})
		if !served {
			return nil, fmt.Errorf("required Flux API %s/v1 is not served", name)
		}
	}
	classes := registrar.client.Resource(schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingressclasses"})
	class, err := classes.Get(ctx, "traefik", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("required supplied Traefik ingress class unavailable: %w", err)
	}
	controller, _, _ := unstructured.NestedString(class.Object, "spec", "controller")
	if controller != "traefik.io/ingress-controller" {
		return nil, fmt.Errorf("ingress class has another controller")
	}
	return append(result, Dependency{Name: "traefik", UID: string(class.GetUID()), Mode: "supplied"}), nil
}
