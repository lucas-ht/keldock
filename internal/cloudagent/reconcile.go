package cloudagent

import (
	"context"
	"fmt"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
)

func connectionID(item cloud.Intent, items map[string]cloud.Intent) string {
	switch {
	case item.Resource != nil:
		return item.Resource.ConnectionID
	case item.Binding != nil:
		if resource := items[item.Binding.ResourceID].Resource; resource != nil {
			return resource.ConnectionID
		}
	case item.Domain != nil:
		return item.Domain.Request.ConnectionID
	case item.Backup != nil:
		if resource := items[item.Backup.ResourceID].Resource; resource != nil {
			return resource.ConnectionID
		}
	}
	return ""
}

// Reconcile never accepts arbitrary manifests, IAM, shell commands or external targets.
func (driver *Driver) Reconcile(ctx context.Context, policy cloud.Policy, item cloud.Intent, items map[string]cloud.Intent) cloud.Evidence {
	if item.Scope != policy.Scope || item.Generation < 1 || item.Revision < 1 {
		return evidence("failed", "InvalidCloudOwnership", 0)
	}
	if item.Connection != nil {
		return driver.connection(ctx, policy, item)
	}
	parent := items[connectionID(item, items)]
	if parent.Connection == nil || parent.Scope != item.Scope {
		return evidence("failed", "ConnectionUnavailable", 0)
	}
	conn := *parent.Connection
	if item.Action == "disconnected" {
		return evidence("disconnected", "PreviouslyDisconnected", conn.Generation)
	}
	if policy.ValidateConnection(conn) != nil {
		return evidence("failed", "ConnectionOutsideLocalPolicy", conn.Generation)
	}
	if !conn.Enabled || parent.Action != "reconcile" {
		if item.Resource != nil {
			if _, err := driver.stopStorage(ctx, policy, conn, *item.Resource, false); err != nil {
				return evidence("failed", "SuspendProjectionUnavailable", conn.Generation)
			}
		}
		if item.Binding != nil {
			return driver.binding(ctx, policy, conn, item, items, true)
		}
		if item.Domain != nil {
			if err := driver.suspendDomain(ctx, policy, conn, *item.Domain); err != nil {
				return unavailable(err, conn.Generation)
			}
		}
		if item.Backup != nil {
			return driver.backup(ctx, policy, conn, item, items)
		}
		return evidence("suspended", "ReconciliationSuspended_AWSAuthorityRequiresOperatorRevocation", conn.Generation)
	}
	switch {
	case item.Resource != nil:
		return driver.storage(ctx, policy, conn, item)
	case item.Binding != nil:
		return driver.binding(ctx, policy, conn, item, items, false)
	case item.Domain != nil:
		return driver.domain(ctx, policy, conn, item)
	case item.Backup != nil:
		return driver.backup(ctx, policy, conn, item, items)
	}
	return evidence("failed", "UnsupportedCloudIntent", conn.Generation)
}

func (driver *Driver) connection(ctx context.Context, policy cloud.Policy, item cloud.Intent) cloud.Evidence {
	conn := *item.Connection
	if !conn.Enabled || item.Action != "reconcile" {
		return evidence("suspended", "ReconciliationSuspended_AWSAuthorityRequiresOperatorRevocation", conn.Generation)
	}
	if err := policy.ValidateConnection(conn); err != nil {
		return evidence("failed", "ConnectionOutsideLocalPolicy", conn.Generation)
	}
	objects, err := crossplane.ProviderConfigs(policy, conn)
	if err != nil {
		return evidence("failed", "ProviderConfigurationRejected", conn.Generation)
	}
	for _, object := range objects {
		if _, err := driver.apply(ctx, object, item.Generation); err != nil {
			return evidence("failed", "ProviderConfigurationUnavailable", conn.Generation)
		}
	}
	identity := policy.Identities[conn.IdentityRef]
	for _, service := range []string{"s3", "iam", "route53"} {
		if _, err := driver.Probe.Config(ctx, conn, identity, service); err != nil {
			return evidence("failed", awsprobe.Reason(err), conn.Generation)
		}
	}
	report := evidence("ready", "ExactProviderIdentitiesVerified", conn.Generation)
	report.Details["account"] = conn.Account
	report.Details["region"] = conn.Region
	report.Details["credentials"] = "projected-web-identity"
	report.Details["resourcePermissions"] = "verified-per-resource"
	return report
}

func projectRevision(items map[string]cloud.Intent, policy cloud.Policy) int64 {
	var revision int64
	for _, item := range items {
		if item.Scope == policy.Scope && item.Revision > revision {
			revision = item.Revision
		}
	}
	return revision
}

func unavailable(err error, generation int64) cloud.Evidence {
	if err == nil {
		err = fmt.Errorf("not converged")
	}
	return evidence("reconciling", awsprobe.Reason(err), generation)
}
