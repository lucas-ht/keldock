package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DataCheckpoint records a retained allocation's independently surviving identity.
type DataCheckpoint = v1.RetainedDatabaseCheckpoint

// DatabaseJournal reads retained namespace generations and live CNPG identities.
func (r *Registrar) DatabaseJournal(ctx context.Context) ([]DataCheckpoint, error) {
	items, err := r.resource("Namespace", "").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=keldock,keldock.io/allocation"})
	if err != nil {
		return nil, err
	}
	entries := []DataCheckpoint{}
	for _, namespace := range items.Items {
		if !strings.HasPrefix(namespace.GetName(), "kd-") {
			continue
		}
		generation, err := strconv.ParseInt(namespace.GetAnnotations()["keldock.io/database-generation"], 10, 64)
		if err != nil || generation < 1 {
			return nil, fmt.Errorf("invalid retained allocation checkpoint")
		}
		labels := namespace.GetLabels()
		entry := DataCheckpoint{Scope: v1.Scope{OrganizationID: labels["keldock.io/organization"], ProjectID: labels["keldock.io/project"]}, ID: labels["keldock.io/allocation"], Generation: generation}
		object, err := r.client.Resource(schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}).Namespace(namespace.GetName()).Get(ctx, "database", metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			entry.UID = string(object.GetUID())
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// VerifyJournalCoverage rejects uncheckpointed surviving workload namespaces.
func (r *Registrar) VerifyJournalCoverage(ctx context.Context, entries []JournalEntry) error {
	known := map[string]JournalEntry{}
	for _, entry := range entries {
		known[entry.EnvironmentID] = entry
	}
	items, err := r.resource("Namespace", "").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=keldock,keldock.io/environment"})
	if err != nil {
		return err
	}
	for _, namespace := range items.Items {
		if namespace.GetLabels()["keldock.io/allocation"] != "" {
			continue
		}
		entry, ok := known[namespace.GetLabels()["keldock.io/environment"]]
		generation, err := strconv.ParseInt(namespace.GetAnnotations()["keldock.io/generation"], 10, 64)
		if !ok || err != nil || generation > entry.Generation {
			return fmt.Errorf("surviving namespace %s lacks a current durable checkpoint", namespace.GetName())
		}
	}
	return nil
}
