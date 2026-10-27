# 0002: State and reconciliation

Status: accepted, 2026-10-27; core contracts and local runtime implemented.
See the [first runtime profile](0004-first-runtime-profile.md) for evidence and remaining gates.

## Context

A UI, CI job or AI session can disappear after requesting work. Controllers and external
services retry independently. Git writeback, duplicated ownership and stale observations
would make it hard to determine what was approved and what actually ran.

## Decision

Use control-plane PostgreSQL as the durable authority for approved desired state,
immutable release/deployment selection, operation history and ownership. Validate a
transactional outbox and leased-worker baseline before adding a workflow engine.

Publish compiled deployment configuration as immutable OCI bundles. The agent projects
approved revisions into the cluster. A trusted registrar owns namespace/identity/guardrail
and Flux registration; Flux owns workloads through application-scoped identities.
Optional Crossplane owns only approved cloud resources assigned to its compositions.
Specialist controllers own their service internals. Each object has one active writer.

Cluster projections are not a second editable authority. Future standalone-controller
mode may use operator-owned CRDs, but must exclude the managed-mode agent for those
objects. Controller installations stay operator-owned until explicitly delegated.

Tie observations to desired generations and actual reconciled revisions. Persist accepted
work independently of clients; retries recover the receipt. Record partial effects and
use idempotency and fencing rather than claiming exactly-once external execution.

## Consequences

Application repositories need no deployment Git repository or generated-manifest commits.
Control-plane backups must cover state, identities and referenced artifacts. Restore and
owner transfer need fencing/reconciliation with surviving clusters before writes resume.

Retained resources live beyond preview deletion. No agent reconnect, delayed webhook or
old worker can select an obsolete generation. Runtime tests must prove these properties;
the design and repository checks do not. See [delivery](../architecture/delivery.md) and
[security and operations](../architecture/security-and-operations.md).
