# Security and operations

Status: design requirements. No security, availability or recovery guarantees have been
validated for a runtime in this repository.

## Trust boundaries

| Boundary                      | Required behavior                                                                                                       |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Public API, UI and MCP        | Authenticate human/service identities; enforce organization/project/environment policy for every lookup and mutation    |
| GitHub and user CI            | Verify webhook signatures, OIDC claims and artifact origin separately; authorize repository, workflow, event and target |
| Durable workers               | Execute only admitted revisions with bounded retries, idempotency, leases and recorded partial effects                  |
| Cluster agent                 | Enroll once, rotate/revoke identity, use outbound authenticated transport and fetch only authorized desired state       |
| Environment registrar         | Own namespaces, guardrails, identities and Flux registration; do not accept application-granted authority               |
| Workload reconciliation       | Use scoped Flux identities with enforced namespace, network and resource boundaries                                     |
| Capability/cloud provisioning | Separate privileged identities and approved profiles; never grant raw provider/IAM/controller access to applications    |

One-organization v1 still needs explicit ownership and negative cross-project tests.
M1 chooses authentication/session/token details before implementation. Authorization
decisions must come from verified identity and server policy, not caller-provided
ownership fields, UI controls, skill instructions or tool annotations.

## Secrets and supply chain

Store and transport credentials through supported secret facilities with scoped access,
rotation, redaction and independent recovery. Preserve secret references in application
definitions and audit. Do not send plaintext cluster/provisioning credentials to CI,
MCP output or assistant context. Logs and repository content are untrusted input.

Pin dependencies and deployment artifacts, verify provenance, define supported versions,
and isolate image/bundle writers and readers. Supplied installations retain their owners.
Never turn an arbitrary chart or executable plugin into an approved capability just
because it can install successfully.

## Durable state and failure

Commit desired state and dispatch atomically. Store operation IDs, desired generations,
attempts, lease ownership and effect inventories. Workers and reconcilers must tolerate
duplicate delivery and interruption; exactly-once external execution is not assumed.
Check generation/fencing before effects and reject stale completion where possible.

Cancellation is a request to stop or converge from recorded effects, not proof of an
instant rollback. Client timeout is not cancellation. Reconnection cannot resurrect a
deleted preview or replace a newer selection. Disconnection makes observations stale;
it does not authorize deletion of the last approved workloads.

Audit actor, client, target, immutable revision, approval, result and correlation IDs.
Distinguish desired, accepted, observed and Ready. Bound diagnostic output and retention,
and surface permission failures, unavailable dependencies and partial effects honestly.

## Recovery and ownership

Back up control-plane state/history, inventory, necessary secret recovery material and
referenced OCI artifacts. Test restoration and reconciliation with surviving clusters.
After restoring older state, fence writes until generations and ownership agree; a
database restore must not replay stale destructive work.

Back up application data through each capability's tested procedure. A healthy database
or completed backup request does not prove recoverability. Record backup/WAL freshness,
perform isolated restores and measure recovery time/data loss in the supported profile.
Release rollback does not reverse database migrations or cloud effects.

Keep retained resources outside disposable preview ownership trees. Removal inventories
workloads, bindings, persistent resources and shared dependencies independently. Owner
transfer requires one active writer and preserved resource identity. Bootstrap and
break-glass recovery must remain possible without the control-plane UI.

## Acceptance and limits

Exercise wrong-project access, overprivileged workload requests, revoked credentials,
stale approvals, duplicate events, lost receipts, delayed generations, worker/agent
restart, partial provisioning, disconnected cleanup, retained data and restored state.
Extend tests as real integrations ship; repository lint is not evidence for these cases.

Document minimum resources and failure domains. A namespace is not a hostile-tenant
sandbox, and an all-in-one deployment is not HA. Public untrusted compute and commercial
hosting require separate isolation and operational evidence.

See [validation policy](../validation/README.md), [hosted requirements](hosted-control-plane.md),
and [security reporting](../../.github/SECURITY.md).
