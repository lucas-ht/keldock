package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func (driver *Driver) suspendDomain(ctx context.Context, policy cloud.Policy, conn cloud.Connection, claim cloud.DomainClaim) error {
	client := driver.Dynamic.Resource(schema.GroupVersionResource{Group: "route53.aws.m.upbound.io", Version: "v1beta1", Resource: "records"}).Namespace(policy.Identities[conn.IdentityRef].Namespace)
	for _, suffix := range []string{"-address", "-marker"} {
		id := claim.ID + suffix
		actual, err := client.Get(ctx, crossplane.Name(claim.Scope, id, "Record"), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if actual.GetLabels()["keldock.io/cloud-intent"] != id || actual.GetLabels()["keldock.io/project"] != policy.ProjectID || actual.GetLabels()["keldock.io/organization"] != policy.OrganizationID {
			return fmt.Errorf("DNS projection owner conflict")
		}
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]string{"resourceVersion": actual.GetResourceVersion()}, "spec": map[string]any{"managementPolicies": []string{"Observe"}}})
		if _, err := client.Patch(ctx, actual.GetName(), types.MergePatchType, patch, metav1.PatchOptions{FieldManager: "keldock-cloud"}); err != nil {
			return err
		}
	}
	return nil
}

func (driver *Driver) disconnectDomain(ctx context.Context, conn cloud.Connection, item cloud.Intent, marker, address map[string]any, state awsprobe.DNSState) cloud.Evidence {
	if !state.Owned && (state.MarkerExists || state.AddressExists) {
		// A rejected claim with no projections owns no external records. Release
		// its inventory reservation without changing the observed foreign writer.
		absent := true
		for _, value := range []map[string]any{address, marker} {
			data, _ := json.Marshal(value)
			object := &unstructured.Unstructured{}
			if err := json.Unmarshal(data, &object.Object); err != nil {
				return unavailable(err, conn.Generation)
			}
			_, err := driver.resource(object).Get(ctx, object.GetName(), metav1.GetOptions{})
			if err == nil {
				absent = false
			} else if !apierrors.IsNotFound(err) {
				return unavailable(err, conn.Generation)
			}
		}
		if absent {
			return evidence("disconnected", "ForeignDNSPreserved", conn.Generation)
		}
	}
	if state.AddressExists && (!state.Owned || !state.Converged) {
		return evidence("failed", "DNSOwnershipDriftRequiresReview", conn.Generation)
	}
	if state.MarkerExists && !state.Owned {
		return evidence("failed", "DNSOwnershipConflict", conn.Generation)
	}
	// Delete the address projection first, even if its AWS record is already absent.
	// Otherwise a surviving controller could recreate records after releasing the claim.
	for index, value := range []map[string]any{address, marker} {
		data, _ := json.Marshal(value)
		object := &unstructured.Unstructured{}
		if err := json.Unmarshal(data, &object.Object); err != nil {
			return unavailable(err, conn.Generation)
		}
		client := driver.resource(object)
		actual, err := client.Get(ctx, object.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if index == 0 && state.AddressExists {
				return evidence("failed", "DNSProjectionMissingRequiresReview", conn.Generation)
			}
			if index == 1 && state.MarkerExists {
				return evidence("failed", "DNSProjectionMissingRequiresReview", conn.Generation)
			}
			continue
		}
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		for key, expected := range object.GetLabels() {
			if actual.GetLabels()[key] != expected {
				return evidence("failed", "DNSProjectionOwnerConflict", conn.Generation)
			}
		}
		uid := actual.GetUID()
		if err := client.Delete(ctx, object.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return unavailable(err, conn.Generation)
		}
		return evidence("reconciling", "RemovingOwnedDNSRecords", conn.Generation)
	}
	return evidence("disconnected", "OwnedDNSRecordsRemoved", conn.Generation)
}
