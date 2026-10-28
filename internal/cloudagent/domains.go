package cloudagent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
	"github.com/lucas-ht/keldock/internal/compiler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (driver *Driver) domain(ctx context.Context, policy cloud.Policy, conn cloud.Connection, item cloud.Intent) cloud.Evidence {
	claim := *item.Domain
	marker, address, err := crossplane.DNSRecords(policy, conn, claim)
	if err != nil {
		return evidence("failed", "DomainOutsideLocalPolicy", conn.Generation)
	}
	configuration, err := driver.Probe.Config(ctx, conn, policy.Identities[conn.IdentityRef], "route53")
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	profile := policy.Profiles[claim.Request.Profile]
	state, err := awsprobe.DNS(ctx, configuration, profile.HostedZoneID, claim.Request.Hostname, crossplane.DNSMarker(claim), profile.IngressIPv4)
	if err != nil {
		return evidence("failed", "DNSOwnershipOrObservationConflict", conn.Generation)
	}
	if item.Action == "disconnect" || item.Action == "disconnected" {
		if state.AddressExists && !state.Converged {
			return evidence("failed", "DNSOwnershipDriftRequiresReview", conn.Generation)
		}
		if !state.MarkerExists && !state.AddressExists {
			return evidence("disconnected", "OwnedDNSRecordsRemoved", conn.Generation)
		}
		if !state.Owned {
			return evidence("failed", "DNSOwnershipConflict", conn.Generation)
		}
		// Keep the marker until the owned address has actually disappeared.
		target := address
		if !state.AddressExists {
			target = marker
		}
		data, _ := json.Marshal(target)
		object := &unstructured.Unstructured{}
		_ = json.Unmarshal(data, &object.Object)
		client := driver.resource(object)
		actual, err := client.Get(ctx, object.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return evidence("failed", "DNSProjectionMissingRequiresReview", conn.Generation)
		}
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		if actual.GetLabels()["keldock.io/cloud-intent"] != object.GetLabels()["keldock.io/cloud-intent"] {
			return evidence("failed", "DNSProjectionOwnerConflict", conn.Generation)
		}
		uid := actual.GetUID()
		if err := client.Delete(ctx, object.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return unavailable(err, conn.Generation)
		}
		return evidence("reconciling", "RemovingOwnedDNSRecords", conn.Generation)
	}
	if item.Action == "retain" {
		return evidence("retained", "DNSClaimRetained", conn.Generation)
	}
	markerObject, err := driver.apply(ctx, marker, item.Generation)
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	if !state.Owned || !ready(markerObject) {
		return evidence("reconciling", "ClaimingDNSOwnership", conn.Generation)
	}
	addressObject, err := driver.apply(ctx, address, item.Generation)
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	report := evidence("reconciling", "DNSRecordConverging", conn.Generation)
	report.Details["ownershipVerified"] = "true"
	if !state.Converged || !ready(addressObject) {
		return report
	}
	report.Details["dnsReady"] = "true"
	report.Reason = "CertificatePending"
	certificates := driver.Dynamic.Resource(schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}).Namespace("ke-" + strings.TrimPrefix(claim.Request.EnvironmentID, "env-"))
	certificate, err := certificates.Get(ctx, compiler.CertificateName(item.ID), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return report
	}
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	issuer, _, _ := unstructured.NestedString(certificate.Object, "spec", "issuerRef", "name")
	domains, _, _ := unstructured.NestedStringSlice(certificate.Object, "spec", "dnsNames")
	if issuer != profile.IssuerRef || len(domains) != 1 || domains[0] != claim.Request.Hostname {
		return evidence("failed", "CertificateOwnerConflict", conn.Generation)
	}
	conditions, _, _ := unstructured.NestedSlice(certificate.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == "Ready" && condition["status"] == "True" {
			report.State = "ready"
			report.Reason = "OwnedDNSAndCertificateVerified"
			report.Details["certificateReady"] = "true"
		}
	}
	return report
}
