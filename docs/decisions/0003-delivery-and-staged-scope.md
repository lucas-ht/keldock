# 0003: Delivery and staged scope

Status: accepted, 2026-10-27; implementation pending.

## Context

The long-term vision includes modular infrastructure, agent operation and hosting.
Building a generic extension system, build service and compute provider immediately
would obscure whether the basic application workflow is useful and reliable.

## Decision

Start with one organization, one existing cluster, GitHub Actions and multi-service
monorepos. Users own builds, tests and image publication. A thin Keldock Action submits
complete immutable releases with verified provenance and returns durable operation
receipts. Keldock compiles runtime configuration and observes deployment outcomes.

Use shared product APIs for UI, CI and MCP. Initial MCP supports discovery, diagnostics,
admitted-release deployment to authorized nonproduction targets, and scoped retry/cancel.
Ship skills only for working flows. Production/admin/destructive tool expansion comes
after the corresponding lifecycle and policy evidence.

Separate capability installation, resource allocation and application bindings. Start
with PostgreSQL through a supplied or explicitly bootstrapped CNPG operator and supplied
backup infrastructure. Prove recovery. General remote module administration, Vault and
Temporal follow later; AWS provisioning is an optional extension.

Preserve external-control-plane and organization boundaries early, while postponing
hosted accounts, billing, shared tenancy and provider compute. A modular Go API/worker
and agent/controller manager with PostgreSQL is the provisional runtime baseline; M1
spikes validate choices before dependency pins and packages are introduced.

## Consequences

V1 has a narrow supported matrix and real operational gates. It does not need Temporal,
Crossplane, AWS, Vault, an LLM or a commercial service. Multi-repository releases,
additional CI providers, remote module installers, builders and GPU/model hosting need
separate evidence and scope decisions. See the [implementation plan](../implementation-plan.md)
and [roadmap](../product/roadmap.md).
