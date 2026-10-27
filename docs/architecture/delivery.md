# Applications, releases and delivery

Status: accepted requirements; contracts and runtime implementation pending in M1–M4.

## Application definition

`keldock.yaml` describes a runtime service graph, not executable infrastructure. M1 must
define its versioned schema before shipping examples or an API compatibility promise.

The definition names components, runtime kind, command/arguments, ports, probes, size
profile, routes, dependency ordering, allowed communication, and resource/secret bindings.
Supported initial kinds are HTTP services, workers, schedules, and bounded release jobs.
Build contexts, Dockerfiles, build arguments, tests and publication remain CI inputs.

Reject unknown fields, duplicate identities/routes, missing bindings, incompatible
profiles, rollout cycles, raw Kubernetes manifests, arbitrary IAM/provider policy,
namespace/RBAC choices and privilege-bearing pod configuration. Runtime communication
cycles are different from prerequisite-order cycles. Do not silently omit a component.

Separate required capabilities from installed implementation details. An app requests an
approved database profile and binding; it cannot select a privileged controller identity
or grant itself access to another project.

## Release and deployment identity

A release is immutable and includes the complete definition/source snapshot, every
component's image digest, and provenance. Preserve repository IDs, source SHA, tested
merge SHA when applicable, CI run/attempt, and the original evidence for reused images.
Multi-repository selection follows the first monorepo flow.

A deployment fixes release ID, environment/configuration revision, resource/binding
revisions, desired generation, and rendered OCI bundle digest. Stable operation and
attempt identities distinguish retry, rollback, supersession and cancellation.

No tag resolution, implicit latest component, partial image map, or client-provided
"verified" flag is accepted. Validate registry pull access separately from CI publication
access. Promotion reuses the same images, with explicitly selected environment inputs.

## GitHub integration and CI

Each self-hosted installation owns its GitHub App and credentials. Guide manifest-based
registration and support an existing-App path. App installation is repository access,
not human login. Keep human authorization independent.

Request minimum permissions for selected repository metadata/content, events, Checks
and Deployments. Workflow/content writes are optional only for explicitly requested
setup PRs; otherwise provide a patch. Never rewrite existing CI unnecessarily.

User CI builds, tests, and publishes. The thin Action submits complete prebuilt release
data through OIDC and can wait for readiness or return a receipt. It invokes no builder,
test command, image publisher or cluster CLI. Builds receive no GitHub App private key,
provisioning credentials or cluster credentials.

Validate OIDC issuer, audience, expiry, repository/owner/workflow/event/ref identity and
target policy. Cryptographically verify authorized artifact origin separately; identity
alone does not prove which image was built. Select a provenance mechanism compatible
with supported private repositories before promising the integration.

Authenticate webhooks, deduplicate delivery IDs, persist processing, and recover missed
events with bounded retry/backoff. Re-evaluate repository rename/transfer, App removal,
revoked credentials and out-of-order builds. Fork code cannot obtain privileged access
through pull_request_target. A green CI event alone is not deployment authorization.

## Durable operation and feedback

Commit desired revision and work dispatch atomically. Acceptance returns a stable
receipt, operation/deployment IDs and URLs. Retrying the same uncertain request returns
that receipt; changed intent needs a new revision. Workers and adapters tolerate retries
and expose partial external effects.

CI wait timeout or lost MCP session does not cancel accepted work. Explicit cancellation
stops future steps where possible, inventories completed effects, and reconciles the
chosen final state. Older generations cannot win after a newer selection or teardown.

One durable record drives UI, Action results, Checks, Deployments/status and a single
updated PR comment. Distinguish build success, accepted request, provisioning, rollout,
Ready, failure, canceled, retained and disconnected outcomes. Do not turn stale health
into fresh success; report observation age and expected generation.

## Compilation and Flux

The trusted compiler emits an allowlisted, immutable OCI configuration bundle. Keep
application image publication and deployment-bundle write permissions separate. There
is no application Git writeback or generated deployment repository.

The registrar owns namespaces, guardrails, identities, sources and Kustomizations. Flux
uses a dedicated application-scoped identity to reconcile only permitted workloads.
It cannot change its own registration, another namespace, or shared controllers.

Pin source and applied bundle revisions and require observed generations. Dependency
ordering must reference the selected release, not any old successful prerequisite.
Service readiness includes actual expected workloads; a CronJob spec being healthy is
different from proving a scheduled execution. Record the object versions used for health.

## Jobs, rollback and previews

Use stable release-job operation identities, bounded retries/timeouts, and explicit
rerun policy. Applications own idempotent migration behavior and compatibility during
rolling version skew. A completed no-op fixture is not proof of database side effects.

Rollback selects a previous immutable release/configuration and records a new generation.
It does not undo migrations, restore data, or reverse cloud side effects automatically.
Multi-service rollout is not a cross-service transaction.

One preview is associated with a stable project/repository/PR identity. New commits
update expected source, and authorized CI admission supplies the release. The first
slice is stateless; M6 adds isolated PostgreSQL. Optional M5 adds object storage.

Require namespace isolation, quotas, network policy, explicit data/identity scope and
operator-controlled routing. Never implicitly reuse production data, secrets or bindings.
Reject a preview when a required capability cannot be isolated safely.

PR closure/expiry revokes stale selection and temporary bindings and prunes temporary
workloads/routes. Retained resources remain visible outside disposable namespace
ownership trees. Teardown runs without a cleanup CI workflow and cannot be reversed by
a delayed event, old worker or reconnecting agent.

See [primary sources](../references.md) and [security](security-and-operations.md).
