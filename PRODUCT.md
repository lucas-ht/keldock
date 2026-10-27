# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Next.js and shadcn/ui are explicitly requested for the product UI. Go owns the
authorized API, durable worker and outbound cluster agent; PostgreSQL owns durable
state. Current pins and implementation evidence live in the repository tooling and
runtime documentation.

## Users

Individuals and small teams operating applications on their own infrastructure,
with a path to larger organizations. The first UI prioritizes deployment and environment
health; users, cluster enrollment and MCP access have dedicated administration screens.
This priority was confirmed by the user for M2.

## Product Purpose

Deploy and operate complete applications on customer-controlled infrastructure through
one authorized control plane that people, CI and AI clients can use. An operator should
be able to identify the selected release, understand current health, diagnose failure,
and manage access without developer-authored deployment manifests.

## Positioning

A FOSS core that works without a paid account or enterprise identity provider.
Customer-controlled infrastructure comes first. A future hosted control plane runs
the same core; managed infrastructure and hosting retain separate evidence gates.

## Operating Context

The first product targets one organization, one existing Kubernetes cluster, GitHub
delivery and multi-service previews. User CI owns builds, tests and image publication.
Keldock admits immutable releases, publishes OCI workload bundles and uses a trusted
registrar with scoped Flux reconciliation. Agents connect outbound to the API.

## Capabilities and Constraints

M1 implements and locally tests the Go API, PostgreSQL dispatch, OCI compilation,
outbound agent, multi-service health, preview teardown and isolation. M2 adds proper
human authentication, user management, single-use enrollment/revocation, a working
Next.js UI and authenticated MCP reads managed through that UI.

Optional application PostgreSQL, general module installation, AWS provisioning,
production promotion, hosted tenancy and hosting are separate milestones. Do not
present an unimplemented capability as available. Skills and MCP never grant authority
that the API would deny.

## Brand Commitments

The product name is Keldock. Preserve the existing AGPLv3 license. Use direct, factual
language and distinguish acceptance, current readiness and stale observations.
There is no existing logo, palette or implemented UI to preserve in this checkout.

## Evidence on Hand

Architecture and roadmap: docs/architecture.md and docs/implementation-plan.md.
Executable evidence: test/runtime and docs/validation/README.md. Current local fixtures
are synthetic and must be identified as such; no customer counts, testimonials,
production availability or live GitHub results may be invented.

## Product Principles

- Make deployment and environment state understandable before administrative detail.
- Keep customer ownership and the complete self-hosted FOSS core intact.
- Enforce authorization and credential revocation at the shared API boundary.
- Report the exact selected revision and observation freshness.
- Show only working operations and keep failures recoverable and visible.
