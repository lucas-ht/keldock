# Keldock implementation plan

Updated 2026-10-27. Milestones are dependency gates, not calendar commitments.
This is a fresh repository: M0/core M1 and M2 are implemented. By operator decision,
private GitHub Container Registry (GHCR) pulls belong to M3; additional registry
profiles follow later. Moving this gate does not claim private pulls were validated.
No previous prototype tests, schemas, or live-cloud results are claimed as Keldock evidence.

## First-release target

An external individual or small team follows the documentation, installs into one
existing customer-controlled cluster, connects GitHub, deploys a complete application,
opens and removes an isolated preview, promotes a tested release, and recovers from a
failure without losing retained data. The supported nonproduction flow works through
MCP and versioned skills as well as the UI.

Core delivery works without an application database capability. Enabling the tested
PostgreSQL profile adds a real migration, scoped bindings, isolated data previews,
backup, and an isolated restore using a supplied destination and identity. Neither
automated AWS provisioning, a paid Keldock account, enterprise SSO, Vault, nor Temporal
is an alpha prerequisite.

Users retain CI builds/tests/publication. Application repositories need only source/build
inputs, keldock.yaml, and a thin submission integration; no Flux/Kubernetes deployment
files, Helm/Terraform, cluster credentials, separate deployment Git repository, or
generated-manifest writeback. A one-container demonstration is insufficient.

## Milestones

| Milestone | Deliverable                                             | Dependencies               | Status                                   |
| --------- | ------------------------------------------------------- | -------------------------- | ---------------------------------------- |
| M0        | Repository, design and executable repository checks     | None                       | Implemented by this bootstrap            |
| M1        | Core contracts and compatibility spikes                 | M0                         | Core implemented; extension gates remain |
| M2        | Durable API/agent, auth, console and MCP reads          | Core M1                    | Implemented                              |
| M3        | GitHub integration, GHCR, thin Action and feedback      | M2                         | Pending                                  |
| M4        | Stateless previews, bounded MCP writes and skills       | M3                         | Pending                                  |
| M5        | Optional AWS resource automation                        | Relevant M1 evidence, M4   | Optional future track                    |
| M6        | PostgreSQL capability, onboarding and self-hosted alpha | M4, PostgreSQL M1 evidence | Pending                                  |
| M7–M10    | More clusters/modules, hosted control plane and hosting | Separate evidence gates    | Later                                    |

Each implemented milestone needs a usable status/failure surface, cancellation semantics,
and updated evidence. Do not defer all operational work to the final packaging milestone.

## M0: Repository foundation

Deliver the existing-license product repository, Keldock identity, scoped agent and
contribution guides, pinned tooling/actions/lockfile, CI, architecture, decisions, and
this plan. `task doctor` proves repository properties and tests the validation tools.
No server, agent, schema compatibility promise, chart, MCP service, or cloud capability
is shipped by M0. Publication is a separate decision.

## M1: Contracts and bounded spikes

1. Define versioned application/release/environment/deployment, organization/project,
   resource/binding, capability installation, operation, enrollment, and observation
   contracts. Preserve stable IDs, exact targets, desired generations and ownership.
   Use conformance tests rather than unvalidated example payloads.
2. Define provider-independent prebuilt-release admission: complete digest map,
   definition/source identity, provenance, idempotency, durable receipt, status/URLs,
   bounded wait and submit-only mode. Select evidence that works for supported private
   repositories; OIDC identity alone does not prove artifact origin.
3. In disposable Kubernetes, reconcile a digest-pinned OCI bundle with web/API/worker,
   schedule, and prerequisite job. Verify scoped Flux impersonation, selected-revision
   dependency ordering, health, pruning, network isolation and rollback. Generate Flux
   objects through trusted tooling, with no Git writeback. Introduce task local:test
   only with this runnable fixture.
4. Prove a PostgreSQL outbox/leased-worker default under restart, duplicate delivery,
   lease expiry, cancellation and partial effects. Record guarantees and minimum cost.
   Temporal is an optional engine profile, not a default installation prerequisite.
5. Select packaging/human authentication and a supported MCP protocol/SDK/auth profile.
   Test supplied dependency detection, not automatic adoption. Define organization
   ownership before persistence; hosted accounts and billing are unnecessary here.
6. Before M6, prove pinned CNPG/operator coexistence, one backup integration, destination
   identity, upgrade/retention and an isolated restore. Before optional M5, separately
   prove actual Crossplane/AWS identity, create/connect/adopt, retention and upgrades.

Record versions, commands, outcomes and limits. Core M1 gates M2; PostgreSQL evidence
gates M6; cloud evidence gates M5. An unsupported extension must not block the supported
core or appear as an available UI capability.

Implemented: [typed core contracts, provenance and compilation](runtime/contracts.md),
PostgreSQL dispatch/recovery, API admission/status, signed event handling, and an outbound
agent with real local Flux delivery. [Local acceptance](runtime/local-development.md)
includes multi-service previews, an API-only image update, prerequisite/readiness failure,
recovery, scoped access, isolation, pruning/rollback, teardown and compiled processes.
The [profile decision](decisions/0004-first-runtime-profile.md) selects packaging;
[ADR 0005](decisions/0005-shared-authentication-and-remote-mcp.md) records implemented
human authentication, agent enrollment and remote MCP. Capability/resource types do
not imply installation/allocation APIs. CNPG/restore and cloud keep separate M6/M5 gates.

This runtime advances M2–M4 without closing every gate. Remaining work includes private
registry pull profiles, thin Action/live GitHub feedback, bounded MCP writes/skills,
general onboarding and data recovery.

## M2: Durable control plane and local delivery

1. Implement the modular Go API/worker/agent/controllers and the first UI surface with
   pinned dependencies, database migrations and an executable development environment.
2. Implement authenticated organization/project/environment ownership, immutable
   records, audited selection, generation concurrency and transactional work dispatch.
3. Compile allowlisted workload configuration and publish immutable OCI bundles. The
   registrar creates protected namespaces, guardrails, identities and Flux sources/syncs.
4. Implement single-use enrollment, outbound sessions, revocation/rotation, approved
   desired-state fetch and revision-bound observations.
5. Expose timeline, component failures and retry. Add authenticated MCP inventory,
   status and bounded redacted diagnostics through the same APIs.

Acceptance: update only the API while pinning other images, fail a prerequisite and
readiness probe, recover each, restart workers/agents during rollout, and reject stale
generations. Workload identities cannot change registration or neighboring environments.
Reject incomplete/tag-only releases, bad provenance and wrong targets. Verify local OCI
pull access and no Git writeback. Accepted work survives a lost submitting client.

Extend task local:test through the real API and agent. The earlier fixture alone does
not close M2. MCP tests include cross-project denial, revoked credentials, structured
errors and stale observations.

Implemented and [locally validated](validation/m2-access-and-console.md): durable local
accounts and project grants; bootstrap/recovery; single-use enrollment, leases, rotation
and revocation; a health-first Next.js console with people/cluster/MCP administration;
retry/cancel and redacted diagnostics; five read tools behind OAuth project consent.
The SDK flow tests current authority and narrowed access. M2 is accepted within this
local profile. The registry is intentionally unauthenticated; private GHCR acceptance
moves to M3. Assistant-host/skill acceptance is M4.

## M3: GitHub CI integration and feedback

1. Implement installation-owned GitHub App manifest/existing-App setup, selected
   repository authorization, key rotation and installation removal.
2. Verify signed webhooks; deduplicate and durably process delivery, recover missed
   events, and handle rate limits/backoff.
3. Implement the thin Action after user CI: verify OIDC and image provenance, submit
   complete immutable releases, return receipts/URLs, and optionally wait for readiness.
4. Report Checks, Deployments/status and one updated PR comment from the durable record.
   Keep build completion, acceptance, rollout readiness and client wait timeout distinct.
5. Implement private GHCR workload-image pulls and the Flux OCI bundle pull path when
   bundles are stored in GHCR. Separate CI publication, control-plane bundle publication
   and cluster read credentials. Test rotation, revocation, denied package access and
   credential projection without exposing secrets in releases, diagnostics or MCP.

Acceptance uses a dedicated real test App/repository. Exercise revoked access, repository
rename/transfer, replay, out-of-order build results, lost acceptance response, runner
termination, and reporting outages. Distinguish PR head from tested merge SHA. No
privileged submission or secret access through untrusted pull_request_target code.
Promotion/retry reuse images; no builder, test runner, or image publisher runs in Keldock.
Local webhook fixtures do not replace live GitHub integration evidence.

M3 supports GHCR first. AWS/ECR, Docker Hub and private Harbor-style registries remain
later profiles, with their own authentication, refresh, TLS and access tests. Keep the
internal credential boundary extensible without advertising unimplemented adapters.

### Live acceptance prerequisites

| Input                                                      | Required scope                                                                                                                            |
| ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| GitHub owner and disposable private application repository | Operator can install the App, run Actions and create/update/close test PRs; use synthetic data and the multi-service fixture              |
| Installation-owned GitHub App                              | Manifest registration and selected-repository installation; private key and webhook secret stay in private installation storage           |
| Reachable development HTTPS origin                         | GitHub can deliver webhooks and Actions can submit; a temporary tunnel to the disposable local installation is sufficient for development |
| GHCR namespace and package access                          | CI publishes prebuilt images; the cluster has a separate read credential; a GHCR-hosted bundle publisher additionally needs write access  |
| CI provenance signing material                             | Trusted workflow signs the existing release contract; the operator registers its public key, with no enterprise attestation dependency    |

Prepare App registration, the Action, durable feedback and registry tests locally before
requesting these credentials. App setup can obtain its own key/webhook secret through
the manifest flow; the operator completes registration and chooses repositories.
Repository API access and GHCR package access are distinct authorization boundaries.
The proposed App permissions are Metadata/Contents/Actions read and Pull requests/Checks/
Deployments write; automatic setup PRs would require separate source/workflow authority.

Actions use GITHUB_TOKEN for image publication and OIDC for Keldock submission. External
GHCR readers use a PAT classic with read:packages and actual package access under the
[current registry profile](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).
Do not assume an App installation token or an expired workflow token can serve as a
durable registry credential. If Keldock publishes its own bundles to GHCR, provision a
separate write:packages credential for that publisher, not for workload image pulls.

Use the existing keldock-m1 target for Kubernetes acceptance. A separate workload domain
is needed only to expose previews externally; the local fixture can validate HTTP using
its explicit test Host header. Live tests need the Action's code accessible from the
test repository; public publication is not required if private Action access is configured.

## M4: Previews, MCP writes and skills

Implement stable PR/environment mapping, quotas/network isolation, explicit preview
profiles and operator-controlled domains. Handle updates, expiry, closure, cancellation,
supersession, retained resources and disconnected clusters. No late build or reconnect
may resurrect a removed environment. Cleanup does not require a CI cleanup workflow.

Expose MCP selection of already admitted releases for permitted nonproduction targets,
and scoped retry/cancel. Publish portable, versioned preview and diagnosis skills only
with real supported tools. Administrative grants, production promotion, module installs,
cloud adoption, data deletion and arbitrary shell access stay outside the v1 MCP surface.

Prove the workflow with an SDK client and one documented assistant host: policy parity
with UI, no provenance bypass, bounded diagnostics, explicit failure, idempotent receipts,
stale request denial and continuation after session loss. Test the skills against the
same workflow. Assistant prose is not evidence of deployment success.

## M5: Optional AWS extension

Implement approved CloudConnections, domains, object/backup storage, workload identity,
adoption and retention only with scoped live evidence. This can follow alpha and must
not add AWS or Crossplane to the minimum stack. See the
[extension acceptance](product/roadmap.md#m5-optional-aws-resources).

## M6: PostgreSQL and external-operator alpha

1. Package working all-in-one control-plane/agent and supplied-dependency profiles,
   plus explicit optional installation of the tested CNPG integration.
2. Complete local administrator authentication/recovery, HTTPS checks, GitHub setup,
   capability inventory, environment UI, and MCP/skills onboarding without enterprise SSO.
3. Ship the web/API/worker/schedule/migration/PostgreSQL acceptance application. Prove
   scoped database bindings, isolated previews, retained-resource visibility and upgrade.
4. Use a supplied compatible backup destination/identity. Prove base backup, expected
   WAL health and an isolated restore; automatically provisioning storage is M5.
5. Test control-plane backup/restore, outage/reconnect, enrollment revocation, artifact
   retention, upgrade/migration and uninstall while preserving retained resources.
6. Publish supported versions, measured minimum resource requirements, limitations,
   contribution/security reporting, release artifacts and working skills.

Exit: a fresh operator completes the documented journey without personal developer
accounts/private services, automatic AWS provisioning, paid accounts, Vault or Temporal.
Core delivery works without optional PostgreSQL; the database profile proves recovery.
Promotion uses identical images and no developer-authored Flux files/commands.
Do not claim single-node HA or hostile-tenant isolation. Publication requires explicit
approval after evidence; optional integrations are advertised only after their own gates.

## Later work and prerequisites

The [expansion roadmap](product/roadmap.md) covers multiple clusters, delegated module
lifecycle, hosted control plane, and hosting. Standalone-controller packaging follows
alpha. Hosted operation need not wait for a broad module catalogue. Managed builders
and GPU/model hosting require separate proposals.

Core runtime work needs disposable cluster/registry capacity, an operator-owned preview
domain, a GitHub test App/repository, CI image publication/pull access, reachable HTTPS
endpoints, an MCP client/host, and a supplied backup destination/identity. Optional cloud
work needs approved disposable resources and exact IAM/target review. None authorizes
modifying an existing production installation.
