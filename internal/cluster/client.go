// Package cluster projects approved revisions through a trusted registrar and scoped Flux identities.
package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ErrOwnership prevents implicit adoption and competing writers.
var ErrOwnership = errors.New("object has another owner or a newer desired generation")

// Registrar owns protected substrate and Flux configuration, never application builds.
type Registrar struct {
	client           dynamic.Interface
	Repository       string
	PlainHTTP        bool
	CredentialsFile  string
	JournalNamespace string
}

var resources = map[string]schema.GroupVersionResource{
	"Deployment":     {Group: "apps", Version: "v1", Resource: "deployments"},
	"Job":            {Group: "batch", Version: "v1", Resource: "jobs"},
	"CronJob":        {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"Service":        {Version: "v1", Resource: "services"},
	"Ingress":        {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"Namespace":      {Version: "v1", Resource: "namespaces"},
	"ServiceAccount": {Version: "v1", Resource: "serviceaccounts"},
	"Secret":         {Version: "v1", Resource: "secrets"},
	"ResourceQuota":  {Version: "v1", Resource: "resourcequotas"},
	"Role":           {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
	"RoleBinding":    {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	"NetworkPolicy":  {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
	"OCIRepository":  {Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"},
	"Kustomization":  {Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
}

// New uses explicit kubeconfig/context, or the current Pod's in-cluster identity.
// It never falls back from a supplied file to the user's current default context.
func New(kubeconfig, kubeContext, repository string, plainHTTP bool) (*Registrar, error) {
	var config *rest.Config
	var err error
	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		if kubeContext == "" {
			return nil, fmt.Errorf("explicit kubeconfig requires an explicit context")
		}
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, &clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("cluster configuration: %w", err)
	}
	config.Timeout = 15 * time.Second
	config.UserAgent = "keldock-registrar/v1alpha1"
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Registrar{client: client, Repository: repository, PlainHTTP: plainHTTP, JournalNamespace: "keldock-system"}, nil
}

func (registrar *Registrar) resource(kind, namespace string) dynamic.ResourceInterface {
	resource := registrar.client.Resource(resources[kind])
	if kind == "Namespace" {
		return resource
	}
	return resource.Namespace(namespace)
}

func (registrar *Registrar) apply(ctx context.Context, object any, environment string, generation int64) error {
	var last error
	err := wait.ExponentialBackoffWithContext(ctx, wait.Backoff{Duration: 20 * time.Millisecond, Factor: 2, Jitter: 0.1, Steps: 6}, func(ctx context.Context) (bool, error) {
		last = registrar.applyOnce(ctx, object, environment, generation)
		if apierrors.IsConflict(last) || apierrors.IsAlreadyExists(last) {
			return false, nil
		}
		return last == nil, last
	})
	if wait.Interrupted(err) && ctx.Err() == nil {
		return last
	}
	return err
}

func (registrar *Registrar) applyOnce(ctx context.Context, object any, environment string, generation int64) error {
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	var desired unstructured.Unstructured
	if err := json.Unmarshal(data, &desired.Object); err != nil {
		return err
	}
	kind := desired.GetKind()
	if _, ok := resources[kind]; !ok || kind == "Deployment" || kind == "Job" || kind == "CronJob" || kind == "Service" || kind == "Ingress" {
		return fmt.Errorf("registrar cannot apply kind %s", kind)
	}
	client := registrar.resource(kind, desired.GetNamespace())
	existing, err := client.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := client.Create(ctx, &desired, metav1.CreateOptions{FieldManager: "keldock-registrar"})
		return err
	}
	if err != nil {
		return err
	}
	if err := owned(existing, environment, generation); err != nil {
		return err
	}
	updated := existing.DeepCopy()
	mergedLabels := updated.GetLabels()
	for key, value := range desired.GetLabels() {
		mergedLabels[key] = value
	}
	updated.SetLabels(mergedLabels)
	annotations := updated.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	for key, value := range desired.GetAnnotations() {
		annotations[key] = value
	}
	updated.SetAnnotations(annotations)
	// The registrar is the sole specification writer for these owned objects.
	// Preserve controller status/finalizers and namespace lifecycle internals.
	for key, value := range desired.Object {
		if key == "metadata" || key == "status" || key == "apiVersion" || key == "kind" || (kind == "Namespace" && key == "spec") {
			continue
		}
		updated.Object[key] = value
	}
	if reflect.DeepEqual(updated.Object, existing.Object) {
		return nil
	}
	_, err = client.Update(ctx, updated, metav1.UpdateOptions{FieldManager: "keldock-registrar"})
	return err
}

func owned(object *unstructured.Unstructured, environment string, generation int64) error {
	if object.GetLabels()["keldock.io/environment"] != environment || object.GetLabels()["app.kubernetes.io/managed-by"] != "keldock" {
		return ErrOwnership
	}
	previous, err := strconv.ParseInt(object.GetAnnotations()["keldock.io/generation"], 10, 64)
	if err != nil || previous > generation {
		return ErrOwnership
	}
	return nil
}

func (registrar *Registrar) remove(ctx context.Context, kind, namespace, name, environment string, generation int64) (bool, error) {
	for range 3 {
		done, err := registrar.removeOnce(ctx, kind, namespace, name, environment, generation)
		if !apierrors.IsConflict(err) {
			return done, err
		}
	}
	return false, nil
}

func (registrar *Registrar) removeOnce(ctx context.Context, kind, namespace, name, environment string, generation int64) (bool, error) {
	client := registrar.resource(kind, namespace)
	object, err := client.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if err := owned(object, environment, generation); err != nil {
		return false, err
	}
	if object.GetDeletionTimestamp() != nil {
		return false, nil
	}
	uid, version := object.GetUID(), object.GetResourceVersion()
	err = client.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}})
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	return false, err
}
