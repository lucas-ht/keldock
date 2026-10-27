# Expansion roadmap

Updated 2026-10-27. Future work; no dates, pricing or service levels are promised.
The [implementation plan](../implementation-plan.md) owns M0–M6 and current status.

## First-release boundary

One self-hosted organization and cluster, GitHub delivery, multi-service previews,
optional PostgreSQL with supplied backup infrastructure, and bounded MCP/skills access. General
module administration, AWS provisioning, hosted service, and compute fleets are separate
gates. Keep the product FOSS and accessible to individuals and small teams.

## M5: Optional AWS resources

This optional track depends on relevant identity/lifecycle spikes and core delivery;
it may follow M6.

- Implement CloudConnection role setup/verification, region/profile grants, scoped
  provider ownership, revocation, and connection health.
- Add object storage create/connect-existing, exact consumer identities, protected
  inventory, retain/disconnect behavior and explicit adoption.
- Add domain ownership claims, scoped Route53/external DNS, conflict prevention and
  certificate readiness. Existing hosted zones retain their owner.
- Automate backup-store provisioning, archive generations, separate writer/restore
  identities, schedules, backup/WAL observation and CNPG bindings.

Acceptance in approved disposable targets proves allowed/denied access, partial-failure
recovery, preview/chart teardown retention, domain conflict behavior and isolated restore.
Crossplane Ready alone is insufficient. Verify supported provider management policies;
Terraform and Crossplane never simultaneously own the same resource.

## M7: External control plane and additional clusters

Support a customer-operated external control plane/management cluster and outbound
workload agents, then additional clusters. Verify identities, capability discovery,
placement, limits, disconnect/reconnect, mixed versions and provisioning ownership.
Workloads retain their last approved state during control-plane outages.

Multi-repository releases and additional CI/cloud adapters need their own provenance,
baseline-selection and integration evidence; they do not block the external topology.
Standalone-controller distribution needs clean-install, exclusive-authority, upgrade
and recovery tests before it becomes a supported package.

## M8: Managed capability lifecycle

Start remote installation and upgrade with the proven PostgreSQL integration. Require
explicit delegated ownership, approved immutable definitions, and typed API/UI/MCP
requests. Validate one additional integration before freezing a general extension API.

Test supplied/managed/external modes, owner conflicts, dependency readiness, permission
denial, partial failure, restart, upgrade, restore, active consumers, removal and retention.
Usable service and recoverable data define success; a successful chart install does not.

Candidates include secret management and Temporal workflows. Evaluate initialization,
identity, persistence, schema changes, backup/recovery, licenses and support cost per
integration. Customer Temporal stays independent of the internal Keldock worker engine.
Publish skills only for tested APIs. A marketplace, executable community modules and
automatic trust of third-party plugins remain separate decisions.

## M9: Hosted control plane

Depends on M6 and the external-agent part of M7, not every cloud/provider/module.
Operate the same FOSS core while customers retain clusters and workload data. Dedicated
customer instances may precede shared tenancy.

Meet the [hosted gates](../architecture/hosted-control-plane.md): cross-customer denial,
durable recovery, quotas, metering, controlled upgrades, revocation, metadata/retention
disclosure, service objectives, and a tested export/ownership handover. Prove that
self-hosted operation works without subscription validation.

Validate willingness to pay and service costs with external users before setting pricing
or support commitments. Keep a straightforward small-team path; hosted convenience does
not justify withholding core authorization, recovery, or automation from self-hosters.

## M10: Managed infrastructure and hosting

Consider operating infrastructure in customer accounts before offering provider-operated
compute. Each requires explicit scope, provisioning ownership, budget controls, capacity,
tenant isolation, recovery, incident response, billing and support evidence.

Public untrusted workloads, builders, GPU/model serving, and a compute fleet add distinct
sandboxing, scheduling, abuse, and economic requirements. None follows automatically
from a working hosted control plane. Introduce those proposals only when demand and
operational readiness justify them.
