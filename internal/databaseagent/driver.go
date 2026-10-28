// Package databaseagent reconciles only independently delegated CNPG allocations.
package databaseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/lucas-ht/keldock/internal/database"
	databasebackup "github.com/lucas-ht/keldock/internal/databaseagent/backup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Driver holds a provisioning identity separately from API and workload reconciliation.
type Driver struct {
	Client    dynamic.Interface
	archiveMu sync.Mutex
	archive   *databasebackup.Service
}

// New uses an exact external context or the pod's own scoped ServiceAccount.
func New(kubeconfig, contextName string) (*Driver, error) {
	var cfg *rest.Config
	var err error
	if kubeconfig == "" {
		if contextName != "" {
			return nil, fmt.Errorf("context requires kubeconfig")
		}
		cfg, err = rest.InClusterConfig()
	} else {
		if contextName == "" {
			return nil, fmt.Errorf("explicit kubeconfig requires context")
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	}
	if err != nil {
		return nil, err
	}
	cfg.Timeout = 15 * time.Second
	cfg.UserAgent = "keldock-database/v1alpha1"
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Driver{Client: client}, nil
}

var resources = map[string]schema.GroupVersionResource{
	"Namespace":       {Version: "v1", Resource: "namespaces"},
	"Secret":          {Version: "v1", Resource: "secrets"},
	"Pod":             {Version: "v1", Resource: "pods"},
	"Cluster":         {Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"},
	"Backup":          {Group: "postgresql.cnpg.io", Version: "v1", Resource: "backups"},
	"ScheduledBackup": {Group: "postgresql.cnpg.io", Version: "v1", Resource: "scheduledbackups"},
	"ObjectStore":     {Group: "barmancloud.cnpg.io", Version: "v1", Resource: "objectstores"},
	"NetworkPolicy":   {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
}

func (d *Driver) apply(ctx context.Context, a database.Allocation, object map[string]any) (*unstructured.Unstructured, error) {
	return d.applyAs(ctx, a, object, "keldock-database", "keldock.io/database-generation")
}

func (d *Driver) applyAs(ctx context.Context, a database.Allocation, object map[string]any, manager, generationKey string) (*unstructured.Unstructured, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	value := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, &value.Object); err != nil {
		return nil, err
	}
	kind := value.GetKind()
	var client dynamic.ResourceInterface = d.Client.Resource(resources[kind])
	if kind != "Namespace" {
		client = d.Client.Resource(resources[kind]).Namespace(value.GetNamespace())
	}
	labels := value.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for key, val := range ownership(a) {
		labels[key] = val
	}
	value.SetLabels(labels)
	annotations := value.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[generationKey] = strconv.FormatInt(a.Generation, 10)
	value.SetAnnotations(annotations)
	previous, err := client.Get(ctx, value.GetName(), metav1.GetOptions{})
	if err == nil {
		for key, val := range ownership(a) {
			if previous.GetLabels()[key] != val {
				return nil, fmt.Errorf("database resource ownership conflict")
			}
		}
		previousGeneration := previous.GetAnnotations()[generationKey]
		if previousGeneration == "" && manager == "keldock-database-backup" {
			previousGeneration = "0"
		}
		generation, err := strconv.ParseInt(previousGeneration, 10, 64)
		if err != nil || generation > a.Generation {
			return nil, fmt.Errorf("stale database projection")
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	return client.Apply(ctx, value.GetName(), value, metav1.ApplyOptions{FieldManager: manager})
}

func ownership(a database.Allocation) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "keldock", "keldock.io/allocation": a.ID, "keldock.io/organization": a.OrganizationID, "keldock.io/project": a.ProjectID, "keldock.io/environment": a.EnvironmentID}
}

func (d *Driver) backup(ctx context.Context, p database.Policy, a database.Allocation) *database.BackupEvidence {
	d.archiveMu.Lock()
	if d.archive == nil {
		d.archive = databasebackup.New(d.Client, d.applyAs, cluster, network)
	}
	service := d.archive
	d.archiveMu.Unlock()
	return service.Reconcile(ctx, p, a)
}
