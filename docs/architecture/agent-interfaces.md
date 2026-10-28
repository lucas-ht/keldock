# API, MCP and agent skills

Status: six shared inspection tools and three bounded nonproduction mutations are
implemented, together with versioned preview and diagnosis skills.
[The M4 evidence](../validation/m4-previews-and-agents.md) records the tested stateless
profile, real SDK and Codex CLI workflows. [ADR 0005](../decisions/0005-shared-authentication-and-remote-mcp.md)
records the OAuth profile; [ADR 0006](../decisions/0006-bounded-nonproduction-operations.md)
records mutation authority and preview lifecycle.

## Shared behavior

Web UI, CI, MCP and a future CLI call the same authorized product APIs. Admission,
ownership, approval, operation history, resource retention and health are server-owned.
There is no alternative privileged AI execution path or required embedded model provider.

All product functionality should be addressable through versioned APIs over time.
Administrative MCP parity follows the underlying implementation; v1 intentionally
exposes a bounded surface. Package the optional adapter with the control plane where
practical, rather than requiring another service for every interface.

Skills are versioned guidance for discovery, workflow and outcome interpretation. They
do not grant authority, execute a hidden control plane, or replace server policy.

## Implemented bounded surface

The stateless Streamable HTTP adapter uses the official Go SDK v1.8.0 and MCP
2026-07-28. Read tools are list_projects, list_environments, list_releases,
get_environment, get_operation and get_diagnostics. Mutations are deploy_release,
retry_operation and cancel_operation. See the [workflow contract](../runtime/agent-workflows.md)
for exact inputs, receipts, error codes, policy limits and skill installation.

Read-only grants remain read-only. Explicit keldock:operate consent narrows current
operator authority to selected projects. HTTP and MCP call one operation service;
current account/grant/client/endpoint authority is rechecked inside the mutation
transaction. Actor-scoped receipts survive client disconnection, and expected
generations prevent stale selection.

CI owns provenance-checked initial admission. Selection references immutable stored
inputs and rechecks current repository delegation, PR head and target policy. MCP
cannot assert arbitrary images are verified. Production promotion, administrative
grants, module installation, cloud adoption, persistent-data deletion and arbitrary
execution remain outside the surface. The two portable skills guide supported preview
and diagnosis/recovery workflows without changing these boundaries.

## Operation contract

- Typed inputs and structured outputs include stable IDs, exact target, desired revision,
  current state, observation age and bounded error details.
- Return durable acceptance separately from completion. A client timeout or closed AI
  session does not cancel accepted work.
- Scope idempotency to authenticated ownership and request identity. Repeating uncertain
  submission returns the receipt; new intent needs a new revision.
- Status reads recover outcomes without a live connection. Notifications can supplement
  them but cannot be the sole record of progress.
- Bind required approval to exact target and immutable revision; reject stale approval
  and recheck authority before execution.
- Distinguish unsupported, denied, missing prerequisite, stale, partial effect, retryable
  failure and completed outcomes. Bound output, pagination, logs and rate limits.

Follow [MCP tools][tools] and the pinned, tested compatibility profile. Design references
alone do not establish runtime compatibility.

## Authorization and data

Use delegated human or service identities, renewable/revocable credentials and explicit
organization/project/environment scopes. Remote MCP follows the selected supported
[authorization profile][auth]. A local adapter still calls authenticated APIs; a skill
file is not a credential. Validate issuer/audience, target and scope at the server.

Cluster and provisioning credentials stay out of assistant context. Return secret
references, audit actor/client/target/revision/result, and redact bounded diagnostics.
Repository content, logs and tool annotations are untrusted data, not new policy. An AI
client cannot increase its own grants.

Permitted nonproduction requests use the same configured approval policy as UI requests;
they do not need a new server approval solely because the client is AI. Clients can
provide their own consent controls. Dangerous/admin actions retain their explicit
operator boundaries when later exposed.

## Skill packaging and acceptance

Use the portable [Agent Skills format][skills]: focused SKILL.md metadata/instructions,
compatible API/tool versions and small reference files. Ship versioned artifacts without
embedded credentials or installation-specific identifiers. No paid account is required.

Test a real SDK client and at least one documented assistant host. Exercise discovery,
structured failure, denied cross-project access, revoked credentials, stale selection,
idempotent retry, session interruption, bounded diagnostics and denied destructive/admin
operations. Run skill workflows against the same tests. Natural-language success claims
are not runtime evidence.

Future module tools expose typed plan/request, status, upgrade and removal only when
their underlying lifecycle is tested. Approval, ownership and retention stay enforced
outside prompts.

[tools]: https://modelcontextprotocol.io/specification/2026-07-28/server/tools
[auth]: https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization
[skills]: https://agentskills.io/specification
