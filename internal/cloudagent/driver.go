package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Driver has a cluster credential scoped to approved provider/inventory and supplied database APIs.
type Driver struct {
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
	Probe      *awsprobe.Client
}

// New never uses a default context: operators must specify the exact configured target.
func New(kubeconfig, contextName string) (*Driver, error) {
	if kubeconfig == "" || contextName == "" {
		return nil, fmt.Errorf("explicit cloud-agent kubeconfig and context required")
	}
	configuration, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		return nil, err
	}
	configuration.Timeout = 15_000_000_000
	kube, err := kubernetes.NewForConfig(configuration)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(configuration)
	if err != nil {
		return nil, err
	}
	return &Driver{Kubernetes: kube, Dynamic: dynamicClient, Probe: &awsprobe.Client{Kubernetes: kube}}, nil
}

var plurals = map[string]string{"ProviderConfig": "providerconfigs", "Bucket": "buckets", "BucketVersioning": "bucketversionings", "BucketPublicAccessBlock": "bucketpublicaccessblocks", "BucketOwnershipControls": "bucketownershipcontrols", "BucketServerSideEncryptionConfiguration": "bucketserversideencryptionconfigurations", "Role": "roles", "Record": "records"}

func (driver *Driver) resource(object *unstructured.Unstructured) dynamic.ResourceInterface {
	gv := object.GroupVersionKind().GroupVersion()
	return driver.Dynamic.Resource(schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: plurals[object.GetKind()]}).Namespace(object.GetNamespace())
}

func (driver *Driver) apply(ctx context.Context, value map[string]any, generation int64) (*unstructured.Unstructured, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, &object.Object); err != nil {
		return nil, err
	}
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["keldock.io/cloud-generation"] = strconv.FormatInt(generation, 10)
	object.SetAnnotations(annotations)
	client := driver.resource(object)
	current, err := client.Get(ctx, object.GetName(), metav1.GetOptions{})
	if err == nil {
		for key, value := range object.GetLabels() {
			if current.GetLabels()[key] != value {
				return nil, fmt.Errorf("managed resource owner conflict")
			}
		}
		if previous, err := strconv.ParseInt(current.GetAnnotations()["keldock.io/cloud-generation"], 10, 64); err != nil || previous > generation {
			return nil, fmt.Errorf("stale cloud projection")
		}
		if expected := object.GetAnnotations()["crossplane.io/external-name"]; expected != "" && expected != current.GetAnnotations()["crossplane.io/external-name"] {
			return nil, fmt.Errorf("external resource identity changed")
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	return client.Apply(ctx, object.GetName(), object, metav1.ApplyOptions{FieldManager: "keldock-cloud"})
}

func ready(object *unstructured.Unstructured) bool {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	values := map[string]string{}
	for _, raw := range conditions {
		if condition, ok := raw.(map[string]any); ok {
			kind, _ := condition["type"].(string)
			value, _ := condition["status"].(string)
			values[kind] = value
		}
	}
	return values["Ready"] == "True" && values["Synced"] == "True"
}

func evidence(state, reason string, connectionGeneration int64) cloud.Evidence {
	return cloud.Evidence{State: state, Reason: reason, ConnectionGeneration: connectionGeneration, Details: map[string]string{}}
}
