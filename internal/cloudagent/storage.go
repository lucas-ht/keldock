package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func (driver *Driver) storage(ctx context.Context, policy cloud.Policy, conn cloud.Connection, item cloud.Intent) cloud.Evidence {
	resource := *item.Resource
	if resource.ConnectionGeneration != conn.Generation {
		_, _ = driver.stopStorage(ctx, policy, conn, resource, false)
		return evidence("suspended", "ConnectionRevisionRequiresExplicitRefresh", conn.Generation)
	}
	if item.Action == "retain" || item.Action == "disconnect" || item.Action == "disconnected" {
		done, err := driver.stopStorage(ctx, policy, conn, resource, item.Action != "retain")
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		if !done {
			return evidence("reconciling", "RetainingExternalStorage", conn.Generation)
		}
		state := "retained"
		if item.Action != "retain" {
			state = "disconnected"
		}
		return evidence(state, "ExternalStorageRetained", conn.Generation)
	}
	objects, err := crossplane.Storage(policy, conn, resource)
	if err != nil {
		return evidence("failed", "StorageOutsideLocalPolicy", conn.Generation)
	}
	if resource.Mode == "adopt" {
		if err := driver.verifyAdoption(ctx, policy, conn, resource); err != nil {
			return evidence("failed", "AdoptionReviewStaleOrWriterNotReleased", conn.Generation)
		}
		objects[0]["metadata"].(map[string]any)["annotations"].(map[string]string)["keldock.io/adoption-review"] = resource.AdoptionReviewHash
	}
	converged := true
	for _, object := range objects {
		actual, err := driver.apply(ctx, object, item.Generation)
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		converged = converged && ready(actual)
	}
	configuration, err := driver.Probe.Config(ctx, conn, policy.Identities[conn.IdentityRef], "s3")
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	snapshot, err := awsprobe.Bucket(ctx, configuration, conn, resource.ExternalName)
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	if !converged || !snapshot.UsableProfile() {
		return evidence("reconciling", "PrivateVersionedStorageConverging", conn.Generation)
	}
	hash, err := snapshot.Hash()
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	report := evidence("ready", "BucketIdentityAndSecurityVerified", conn.Generation)
	report.Details = map[string]string{"account": snapshot.Account, "region": snapshot.Region, "bucket": snapshot.Bucket, "inventoryHash": hash, "managementOwner": resource.Owner, "dataPath": "not-probed-by-provisioner"}
	return report
}

func (driver *Driver) stopStorage(ctx context.Context, policy cloud.Policy, conn cloud.Connection, resource cloud.Resource, disconnect bool) (bool, error) {
	done := true
	namespace := policy.Identities[conn.IdentityRef].Namespace
	for _, kind := range []string{"Bucket", "BucketVersioning", "BucketPublicAccessBlock", "BucketOwnershipControls", "BucketServerSideEncryptionConfiguration"} {
		object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "s3.aws.m.upbound.io/v1beta1", "kind": kind, "metadata": map[string]any{"name": crossplane.Name(resource.Scope, resource.ID, kind), "namespace": namespace}}}
		client := driver.resource(object)
		actual, err := client.Get(ctx, object.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		labels := actual.GetLabels()
		if labels["keldock.io/cloud-intent"] != resource.ID || labels["keldock.io/organization"] != policy.OrganizationID || labels["keldock.io/project"] != policy.ProjectID {
			return false, fmt.Errorf("managed resource owner conflict")
		}
		previous, err := strconv.ParseInt(actual.GetAnnotations()["keldock.io/cloud-generation"], 10, 64)
		if err != nil || previous > resource.Generation {
			return false, fmt.Errorf("stale retained projection")
		}
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"resourceVersion": actual.GetResourceVersion(), "annotations": map[string]string{"keldock.io/cloud-generation": strconv.FormatInt(resource.Generation, 10)}}, "spec": map[string]any{"managementPolicies": []string{"Observe"}}})
		if _, err := client.Patch(ctx, object.GetName(), types.MergePatchType, patch, metav1.PatchOptions{FieldManager: "keldock-cloud"}); err != nil {
			return false, err
		}
		if disconnect {
			uid := actual.GetUID()
			if err := client.Delete(ctx, object.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
			done = false
		}
	}
	return done, nil
}
