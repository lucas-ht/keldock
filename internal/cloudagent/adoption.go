package cloudagent

import (
	"context"
	"fmt"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (driver *Driver) verifyAdoption(ctx context.Context, policy cloud.Policy, conn cloud.Connection, resource cloud.Resource) error {
	client := driver.Dynamic.Resource(schema.GroupVersionResource{Group: "s3.aws.m.upbound.io", Version: "v1beta1", Resource: "buckets"}).Namespace(policy.Identities[conn.IdentityRef].Namespace)
	actual, err := client.Get(ctx, crossplane.Name(resource.Scope, resource.ID, "Bucket"), metav1.GetOptions{})
	if err == nil && actual.GetAnnotations()["keldock.io/adoption-review"] == resource.AdoptionReviewHash && resource.AdoptionReviewHash != "" {
		return nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	grant, ok := policy.Adoptions[resource.ExternalName]
	if !ok {
		return fmt.Errorf("operator writer release is absent")
	}
	configuration, err := driver.Probe.Config(ctx, conn, policy.Identities[conn.IdentityRef], "s3")
	if err != nil {
		return err
	}
	snapshot, err := awsprobe.Bucket(ctx, configuration, conn, resource.ExternalName)
	if err != nil {
		return err
	}
	hash, err := snapshot.Hash()
	if err != nil {
		return err
	}
	before := resource
	before.Mode = "connect"
	before.Owner = "external"
	before.Generation = resource.AdoptionGeneration
	review, err := cloud.AdoptionReview(before, conn, hash, grant)
	if err != nil || review != resource.AdoptionReviewHash {
		return fmt.Errorf("adoption inventory changed since review")
	}
	return nil
}
