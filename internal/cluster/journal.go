package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// JournalEntry survives environment namespace deletion and control-plane database loss.
type JournalEntry = v1.RecoveryCheckpoint

var journalResource = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

func (r *Registrar) checkpoint(ctx context.Context, entry JournalEntry) error {
	if r.JournalNamespace == "" {
		return fmt.Errorf("durable journal namespace is required")
	}
	if !v1.ValidName(entry.EnvironmentID) || entry.Generation < 1 {
		return fmt.Errorf("invalid journal identity")
	}
	client := r.client.Resource(journalResource).Namespace(r.JournalNamespace)
	name := "keldock-journal-" + entry.EnvironmentID
	for range 5 {
		current, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		creating := apierrors.IsNotFound(err)
		if creating {
			current = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": name, "namespace": r.JournalNamespace}}}
			current.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "keldock", "keldock.io/journal": entry.EnvironmentID})
		} else {
			if current.GetLabels()["keldock.io/journal"] != entry.EnvironmentID || current.GetLabels()["app.kubernetes.io/managed-by"] != "keldock" {
				return ErrOwnership
			}
			var previous JournalEntry
			data, _, _ := unstructured.NestedString(current.Object, "data", "entry.json")
			if err := json.Unmarshal([]byte(data), &previous); err != nil {
				return fmt.Errorf("unreadable durable journal")
			}
			if entry.OrganizationID == "" {
				entry.Scope = previous.Scope
			}
			if previous.Scope != entry.Scope || previous.EnvironmentID != entry.EnvironmentID || previous.Generation > entry.Generation {
				return ErrOwnership
			}
			if previous.Generation == entry.Generation {
				if previous == entry {
					return nil
				}
				if previous.Action != "deploy" || entry.Action != "suspend" || previous.DeploymentID != entry.DeploymentID {
					return ErrOwnership
				}
			}
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := unstructured.SetNestedStringMap(current.Object, map[string]string{"entry.json": string(data)}, "data"); err != nil {
			return err
		}
		if creating {
			_, err = client.Create(ctx, current, metav1.CreateOptions{FieldManager: "keldock-journal"})
		} else {
			_, err = client.Update(ctx, current, metav1.UpdateOptions{FieldManager: "keldock-journal"})
		}
		if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
			continue
		}
		return err
	}
	return errors.New("journal concurrency conflict")
}

// Journal reads retained checkpoints without changing surviving cluster state.
func (r *Registrar) Journal(ctx context.Context) ([]JournalEntry, error) {
	if r.JournalNamespace == "" {
		return nil, fmt.Errorf("durable journal namespace is required")
	}
	items, err := r.client.Resource(journalResource).Namespace(r.JournalNamespace).List(ctx, metav1.ListOptions{LabelSelector: "keldock.io/journal"})
	if err != nil {
		return nil, err
	}
	entries := make([]JournalEntry, 0, len(items.Items))
	for _, item := range items.Items {
		var entry JournalEntry
		data, _, _ := unstructured.NestedString(item.Object, "data", "entry.json")
		if json.Unmarshal([]byte(data), &entry) != nil || entry.EnvironmentID != item.GetLabels()["keldock.io/journal"] || !v1.ValidName(entry.EnvironmentID) || entry.Generation < 1 {
			return nil, fmt.Errorf("invalid cluster checkpoint")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
