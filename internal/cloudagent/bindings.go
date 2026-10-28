package cloudagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/cloud/awsprobe"
	"github.com/lucas-ht/keldock/internal/cloud/crossplane"
)

func consumerRole(policy cloud.Policy, conn cloud.Connection, binding cloud.Binding, consumer cloud.ConsumerIdentity, resource cloud.Resource) (crossplane.ConsumerRole, error) {
	profile, ok := policy.Profiles[resource.Profile]
	if !ok || resource.Scope != policy.Scope || binding.Scope != policy.Scope || resource.ConnectionID != conn.ID || !strings.HasPrefix(binding.EnvironmentID, "env-") {
		return crossplane.ConsumerRole{}, fmt.Errorf("binding outside approved resource")
	}
	sa, arn := cloud.WorkloadIdentity(conn, profile, binding.EnvironmentID, consumer.Component)
	prefix := "projects/" + policy.ProjectID + "/environments/" + binding.EnvironmentID + "/components/" + consumer.Component + "/"
	if consumer.Namespace != "ke-"+strings.TrimPrefix(binding.EnvironmentID, "env-") || consumer.ServiceAccount != sa || consumer.RoleARN != arn || consumer.Prefix != prefix {
		return crossplane.ConsumerRole{}, fmt.Errorf("consumer identity escaped trusted placement")
	}
	return crossplane.ConsumerRole{ID: sa, ARN: arn, Namespace: consumer.Namespace, ServiceAccount: sa, Profile: resource.Profile}, nil
}

func (driver *Driver) binding(ctx context.Context, policy cloud.Policy, conn cloud.Connection, item cloud.Intent, items map[string]cloud.Intent, suspended bool) cloud.Evidence {
	binding := *item.Binding
	resource := items[binding.ResourceID]
	if resource.Resource == nil {
		return evidence("failed", "BindingResourceUnavailable", conn.Generation)
	}
	configuration, err := driver.Probe.Config(ctx, conn, policy.Identities[conn.IdentityRef], "iam")
	if err != nil {
		return unavailable(err, conn.Generation)
	}
	for _, consumer := range binding.Consumers {
		role, err := consumerRole(policy, conn, binding, consumer, *resource.Resource)
		if err != nil {
			return evidence("failed", "BindingOutsideLocalPolicy", conn.Generation)
		}
		if !suspended {
			for _, other := range items {
				if other.Binding == nil || other.Scope != policy.Scope || other.Action != "reconcile" || other.Binding.ConnectionGeneration != conn.Generation {
					continue
				}
				otherResource := items[other.Binding.ResourceID]
				if otherResource.Resource == nil || otherResource.Resource.ConnectionID != conn.ID || otherResource.Resource.ConnectionGeneration != conn.Generation || otherResource.Action != "reconcile" {
					continue
				}
				for _, target := range other.Binding.Consumers {
					otherRole, err := consumerRole(policy, conn, *other.Binding, target, *otherResource.Resource)
					if err != nil {
						return evidence("failed", "BindingOutsideLocalPolicy", conn.Generation)
					}
					if target.RoleARN == consumer.RoleARN {
						if otherRole.Profile != role.Profile && (policy.Profiles[otherRole.Profile].RolePath != policy.Profiles[role.Profile].RolePath || policy.Profiles[otherRole.Profile].BoundaryARN != policy.Profiles[role.Profile].BoundaryARN) {
							return evidence("failed", "ConflictingConsumerAuthority", conn.Generation)
						}
						role.Grants = append(role.Grants, crossplane.Grant{Bucket: otherResource.Resource.ExternalName, Prefix: target.Prefix, Access: other.Binding.Access})
					}
				}
			}
		}
		object, trust, permissions, err := crossplane.Role(policy, conn, role)
		if err != nil {
			return evidence("failed", "ConsumerRoleRejected", conn.Generation)
		}
		actual, err := driver.apply(ctx, object, projectRevision(items, policy))
		if err != nil {
			return unavailable(err, conn.Generation)
		}
		if !ready(actual) {
			return evidence("reconciling", "ConsumerIdentityConverging", conn.Generation)
		}
		if err := awsprobe.Role(ctx, configuration, role.ARN, policy.Profiles[role.Profile].BoundaryARN, trust, "keldock", permissions); err != nil {
			return unavailable(err, conn.Generation)
		}
	}
	state, reason := "ready", "ExactConsumerIdentityAndPolicyVerified"
	if suspended {
		state, reason = "suspended", "ConsumerGrantsSuspended"
	} else if item.Action == "disconnect" || item.Action == "disconnected" {
		state, reason = "disconnected", "ConsumerGrantRemoved"
	} else if item.Action == "retain" {
		state, reason = "retained", "ConsumerGrantRemoved"
	}
	report := evidence(state, reason, conn.Generation)
	if state == "ready" {
		report.Details["identityReady"] = "true"
		report.Details["dataPath"] = "workload-verification-required"
	}
	return report
}
