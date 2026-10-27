# API, MCP and agent skills

Status: shared experimental delivery/status API implemented. MCP adapter and skill
artifacts remain pending; the [first runtime profile](../decisions/0004-first-runtime-profile.md)
selects their protocol/SDK/auth target without claiming runtime compatibility.

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

## V1 surface

| Operation    | Behavior                                                                                        |
| ------------ | ----------------------------------------------------------------------------------------------- |
| Discover     | Paginated authorized projects, environments, admitted releases and capabilities                 |
| Inspect      | Operation/deployment state, observation age, component failure and bounded redacted diagnostics |
| Deploy       | Select an already admitted immutable release for an authorized nonproduction target             |
| Retry/cancel | Apply the existing operation policy, identity, revision and cancellation semantics              |

Exact names/schemas are an implementation deliverable. CI owns the first provenance-checked
release submission. MCP cannot assert that an arbitrary image is verified, and release
reuse still checks current target authorization. Production promotion, privilege grants,
module installation, cloud adoption, persistent-data deletion and arbitrary shell access
are outside the initial tool surface.

The first skills cover preview deployment from an admitted release and diagnosis/recovery
of a failed rollout. Explain missing prerequisites, CI-owned build failures, supported
retry/cancel, and operator decisions. Publish only tools and flows that work.

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

Follow [MCP tools][tools] and choose a pinned protocol/SDK compatibility profile in M1.
Design references do not select a dependency version automatically.

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
