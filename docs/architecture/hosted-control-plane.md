# Future hosted control plane

Status: future design; not part of the first self-hosted release.

## Same core, customer infrastructure

The hosted offering runs the same FOSS control plane and agent protocol. Customers
retain clusters, workload data, supported registry choice and infrastructure accounts.
Paid operation covers availability, upgrades, recovery, support and fleet convenience;
it does not gate the self-hosted API, authorization, MCP, skills or recovery tooling.

An external control plane is a topology. A commercial service adds isolation and
operational commitments. Dedicated customer instances can precede shared tenancy;
neither is proven by a local all-in-one installation.

Administrators enroll outbound agents with explicit capability grants. Kubernetes
credentials stay in the cluster; provisioning uses scoped customer-approved identities.
Enrollment never implicitly adopts existing services or expands grants.

The control plane receives application configuration, metadata, audit and permitted
diagnostics. Document what leaves the cluster, region, access, retention and deletion.
Customer-owned compute does not imply that no data reaches the hosted control plane.

## Decisions required before M2

V1 serves one organization, but durable records and authentication need an explicit
installation/organization owner. Resolve authority from authenticated context, not a
caller-supplied organization ID. Scope resource lookup, idempotency, queues, credentials,
OCI artifact access and audit to that owner.

M1 defines these contracts; no tenant isolation is implemented by this bootstrap. Reserve
the ownership boundary without building a billing system, hosted account service, or
shared-tenant deployment in v1. Self-hosting must not require hosted license validation.

## Disconnection, recovery and exit

Agents and controllers retain the last approved desired state during an outage. Loss
of connection does not authorize deletion or a new deployment. Report freshness; normal
workload execution still depends on customer infrastructure and its external services.

Back up desired state/history, inventories, identities, required secret recovery material
and active/rollback bundle references. Provide a tested versioned export/recovery path.
Move control-plane ownership through a reviewed handover with credential rotation,
generation reconciliation and one active writer. Do not require image rebuilds or
silent infrastructure adoption.

Cluster bootstrap and break-glass recovery remain usable without the hosted dashboard.
Define failure behavior and artifact retention for hosted registries and other service
dependencies. Restore an older database in observation/recovery mode until surviving
cluster generations and ownership have been reconciled.

## Commercial launch gates

- Cross-customer denial across APIs, MCP, agents, storage, queues, registries, logs,
  backups and support tools. Dedicated instances still need isolated credentials.
- Quotas, concurrency/rate limits, resource accounting, audit and noisy-neighbor controls.
- Measured availability objectives, durable work recovery, backup/restore, key rotation,
  incident response and support operations.
- Mixed versions, staged upgrades, disconnect/reconnect, revocation and portable exit.
- Accurate metering/billing and customer-visible limits, costs, ownership and data policy.

See the [roadmap](../product/roadmap.md). Managed customer infrastructure and
provider-operated compute require separate capacity, isolation, abuse, cost and support
evidence. Pricing and service levels remain undecided.
