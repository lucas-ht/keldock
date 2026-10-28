# 0006: Bounded nonproduction operations and stateless preview policy

Status: accepted design; M4 implementation and full acceptance in progress.

## Decision

Preserve one preview identity for each organization/project/repository/PR, including
application renames. Operator-owned preview configuration selects stateless-v1 or
explicitly disables previews. The profile owns namespace guardrails and routing;
application input cannot choose namespaces, credentials or persistent bindings.

The default lifetime is 24 hours and project capacity is ten previews. Supported bounds
are 60 seconds to seven days and one to 50 previews. Cleanup reserves a capacity slot
until deletion is observed, including when the cluster is disconnected. Expiry leaves
a tombstone; subsequent CI, periodic PR snapshots and old agents cannot remove it.
A current operator may explicitly recreate an expired preview after cleanup, using a
matching admitted release. A newer explicit PR reopen can permit fresh CI admission;
ordinary open/synchronize snapshots cannot undo expiry.

HTTP and MCP share one mutation service. The surface reuses a CI-admitted release for
an existing nonproduction target, retries a current failed/canceled deployment, or
cancels the exact current operation. Release IDs reference immutable stored inputs;
clients cannot introduce images, runtime definitions or provenance through this path.
PR source heads must still match. Production mutation and persistent-data destruction
remain excluded, even for administrator-owned MCP connections.

Preserve read-only grants when adding keldock:operate. New write consent is explicit,
project scoped and restricted by the user's current operator role. Refresh retains the
consented permissions. Check account, grant, client and endpoint authority in the same
transaction as desired-state mutation, with stable lock order and revocation fencing.
An MCP grant never becomes an administrative session.

Acquire authorization row locks in account, endpoint, client and grant order. A joined
locking query can acquire a grant first, blocking an administrator who already holds
the endpoint/client while revoking another user's grants. Real PostgreSQL concurrency
tests cover both revocation paths. This follows PostgreSQL's
[consistent lock ordering guidance](https://www.postgresql.org/docs/18/explicit-locking.html#LOCKING-DEADLOCKS).

Bind each intent to stable target IDs, the observed desired generation and an actor-scoped
request key. Identical uncertain retries return the historical durable receipt;
changed intent conflicts. New accepted intents are limited to 30 per minute per actor
within the organization; receipt recovery does not spend that budget. Keep acceptance,
current selection and fresh matching readiness separate after client/session loss.
Cancellation retains completed effects. Legacy session cancellation remains compatible
because its immutable operation ID already binds the target revision; modern UI and MCP
requests additionally provide the generation and request identity explicitly.

Ship portable versioned preview and diagnosis skills for this implemented tool surface.
Skills describe discovery, authority boundaries and outcome interpretation; they cannot
create authority or bypass denials. Their acceptance requires a real SDK flow and a
separately documented assistant host against the same deployment record.

## Evidence and consequences

Existing M2/M3 namespace isolation, resource quotas, immutable deployment compilation,
leases and GitHub lifecycle recovery remain foundations. M4 adds explicit project
policy, transactional selection and scoped write grants. The local profile is stateless;
retained resource types do not imply a persistent resource API or PostgreSQL recovery.

Reproduce checks through task doctor and task local:test. Record live keldock-example
preview updates, failure/recovery, expiry, disconnection and cleanup separately from
controlled upstream and SDK contract tests. Do not count skill format checks or model
prose as runtime proof.

The [MCP tool contract](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)
defines schemas, structured outcomes and advisory annotations; authorization stays in
Keldock. The pinned [Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
supplies the transport and schema validation. The [Agent Skills specification](https://agentskills.io/specification)
defines portable metadata and references; version metadata does not replace behavioral
acceptance.
