package cluster

import (
	"context"
	"fmt"
	"slices"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reconcile registers one approved bundle and prunes retired component registrations.
// Calls are serialized by the agent; every object also rejects older generations and owner conflicts.
func (registrar *Registrar) Reconcile(ctx context.Context, selection compiler.Selection, definition v1.Application, bundle string) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	if !v1.ValidName(selection.Namespace) || !v1.ValidName(selection.EnvironmentID) || !v1.ValidName(selection.DeploymentID) || selection.Generation < 1 {
		return fmt.Errorf("invalid selected environment")
	}
	repository, digest, err := artifacts.SplitReference(bundle)
	if err != nil || repository != registrar.Repository {
		return fmt.Errorf("bundle outside enrolled registry scope")
	}
	if err := registrar.register(ctx, selection); err != nil {
		return err
	}
	source := map[string]any{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "OCIRepository", "metadata": metadata("keldock-source", selection),
		"spec": map[string]any{"interval": "1m", "url": "oci://" + repository, "ref": map[string]any{"digest": digest}, "insecure": registrar.PlainHTTP, "provider": "generic", "suspend": false,
			"layerSelector": map[string]any{"mediaType": artifacts.LayerMediaType, "operation": "extract"}}}
	if err := registrar.apply(ctx, source, selection.EnvironmentID, selection.Generation); err != nil {
		return fmt.Errorf("register source: %w", err)
	}
	order, err := definition.Order()
	if err != nil {
		return err
	}
	for _, name := range order {
		spec := map[string]any{"interval": "1m", "retryInterval": "2s", "timeout": "30s", "path": "./components/" + name, "prune": true, "wait": true,
			"force": false, "suspend": false,
			"deletionPolicy": "WaitForTermination", "targetNamespace": selection.Namespace, "serviceAccountName": "reconciler",
			"sourceRef": map[string]any{"kind": "OCIRepository", "name": "keldock-source"}}
		dependencies := []map[string]any{}
		for _, dependency := range definition.Components[name].DependsOn {
			dependencies = append(dependencies, map[string]any{"name": "keldock-" + dependency, "readyExpr": fmt.Sprintf(
				`dep.metadata.labels['keldock.io/deployment'] == self.metadata.labels['keldock.io/deployment'] && has(dep.status.conditions) && dep.status.conditions.exists(c, c.type == 'Ready' && c.status == 'True') && dep.metadata.generation == dep.status.observedGeneration && has(dep.status.lastAppliedRevision) && dep.status.lastAppliedRevision.endsWith('%s')`, digest)})
		}
		spec["dependsOn"] = dependencies
		object := map[string]any{"apiVersion": "kustomize.toolkit.fluxcd.io/v1", "kind": "Kustomization", "metadata": metadata("keldock-"+name, selection), "spec": spec}
		if err := registrar.apply(ctx, object, selection.EnvironmentID, selection.Generation); err != nil {
			return fmt.Errorf("register component %s: %w", name, err)
		}
	}
	objects, err := registrar.resource("Kustomization", selection.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "keldock.io/environment=" + selection.EnvironmentID})
	if err != nil {
		return err
	}
	for _, object := range objects.Items {
		retained := slices.ContainsFunc(order, func(name string) bool { return object.GetName() == "keldock-"+name })
		if !retained {
			if _, err := registrar.remove(ctx, "Kustomization", selection.Namespace, object.GetName(), selection.EnvironmentID, selection.Generation); err != nil {
				return err
			}
		}
	}
	return nil
}

// Remove prunes Flux workloads before removing their identities and disposable namespace.
func (registrar *Registrar) Remove(ctx context.Context, selection compiler.Selection) (bool, error) {
	namespace, err := registrar.resource("Namespace", "").Get(ctx, selection.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if err := owned(namespace, selection.EnvironmentID, selection.Generation); err != nil {
		return false, err
	}
	objects, err := registrar.resource("Kustomization", selection.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "keldock.io/environment=" + selection.EnvironmentID})
	if err != nil {
		return false, err
	}
	if len(objects.Items) > 0 {
		for _, object := range objects.Items {
			if _, err := registrar.remove(ctx, "Kustomization", selection.Namespace, object.GetName(), selection.EnvironmentID, selection.Generation); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	return registrar.remove(ctx, "Namespace", "", selection.Namespace, selection.EnvironmentID, selection.Generation)
}
