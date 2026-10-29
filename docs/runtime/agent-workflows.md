# Preview and recovery workflows through MCP

The experimental M6 surface uses MCP 2026-07-28, the Go SDK v1.8.0 and server version
0.6.0-alpha.1. Its portable skills are version 1.1.0. The self-hosted endpoint and skills
require no paid Keldock account or embedded model provider. An assistant host may have
its own account requirements.

Modern HTTP clients carry protocol, identity and capabilities on each request,
with matching `MCP-Protocol-Version`, `Mcp-Method` and (for tool calls) `Mcp-Name`
headers. They use `server/discover` instead of a legacy `initialize` handshake.
Browser preflight permits these standard headers. See the supported revision's
[request metadata contract](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http#request-metadata).

## Connect with bounded authority

Enable the endpoint and register/connect a public client as described in
[authentication](authentication.md). The canonical resource is the installation's
HTTPS `/mcp` URL. Default and existing grants remain `keldock:read`. A current operator
or administrator can explicitly consent to `keldock:read keldock:operate` for selected
projects. A viewer cannot delegate writes. Refresh preserves the existing scope;
request new consent to add operation authority.

The same operation service backs console and MCP mutations. Every write checks current
account, endpoint, client and project consent in the mutation transaction. MCP has no
administrative session privileges. Production promotion, capability installation, cloud
adoption, credential administration, persistent-data deletion and shell execution are
outside this tool surface.

## Tools and receipts

| Tool              | Input                                                       | Outcome                                                                                                                        |
| ----------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| list_projects     | none                                                        | Authorized projects and preview policy                                                                                         |
| list_resources    | projectId, optional after                                   | Up to 25 PostgreSQL allocations with retention, backup/WAL and isolated-restore evidence; no credentials or resource mutations |
| list_environments | projectId, optional after                                   | Up to 25 targets; follow nextCursor                                                                                            |
| get_environment   | projectId, environmentId                                    | Current selection, source head, expiry and matching health                                                                     |
| list_releases     | projectId, optional after                                   | Up to 25 admitted releases and immutable digests; follow nextCursor                                                            |
| get_operation     | projectId, operationId                                      | Durable operation and matching observation/freshness                                                                           |
| get_diagnostics   | projectId, environmentId                                    | Bounded redacted current-generation controller evidence                                                                        |
| deploy_release    | projectId, environmentId, releaseId, generation, requestKey | New deployment of admitted inputs for an existing nonproduction target                                                         |
| retry_operation   | projectId, operationId, generation, requestKey              | New generation of the current failed/canceled nonproduction deployment                                                         |
| cancel_operation  | projectId, operationId, generation, requestKey              | Stop future work; retain completed effects and running workloads                                                               |

Mutation success returns `receipt`, containing releaseId, environmentId, deploymentId,
operationId, generation and statusUrl. Preserve all inputs and the requestKey, which
must be 8–128 characters. An identical uncertain retry returns the original receipt,
including after session loss or later supersession. Reusing that key for changed intent
conflicts. New accepted intents are limited to 30 per minute per actor/organization;
receipt recovery does not consume this budget.

Acceptance is not readiness. Read the operation and environment with bounded polling.
Report Ready only when the current selection, receipt and fresh ready observation
agree on deployment and generation. For previews, the source head must also remain
current. A configured CronJob does not prove an execution completed. The environment
`url` opens its console record; `hostname` is the operator-controlled workload route.
Do not invent public DNS, TLS or reachability from a hostname.

The release catalog omits runtime environment values, commands and provenance material.
Release IDs select immutable stored CI inputs. Tools cannot introduce new images or
bypass OIDC/provenance admission. CI must first admit a release matching the current PR
head. Retry may rerun release jobs and cannot repair faulty inputs or reverse effects.

HTTP clients use GET `/v1/projects/{project}/releases` for the catalog and POST
`/v1/projects/{project}/environments/{environment}/deployments` with releaseId/generation.
POST `/v1/projects/{project}/operations/{operation}/retry` and `/cancel` take generation.
Each mutation supplies `Idempotency-Key`; human sessions additionally require the
canonical Origin and session CSRF proof. An explicitly scoped MCP bearer can use the
same mutation endpoints. HTTP stale/conflict responses use status 409 and code conflict;
MCP distinguishes stale_revision in its structured tool result.

## Failure handling

Operation errors are tool results with IsError and a structured `error` containing code/message.
Schema validation rejects unsupported extra arguments before operation handling.

| Code            | Required behavior                                                            |
| --------------- | ---------------------------------------------------------------------------- |
| denied          | Stop; review current role/project/operation consent or placement policy      |
| not_found       | Recheck exact IDs within the authorized project                              |
| invalid_request | Correct the input contract; preserve an uncertain submitted intent           |
| stale_revision  | Inspect current state and resolve intent before any new selection            |
| conflict        | Use the original key only for its identical intent                           |
| quota_exceeded  | Wait for observed cleanup or ask the operator to review capacity             |
| rate_limited    | Recover the identical request later with its original key                    |
| unavailable     | Preserve the exact intent/key; inspect the durable record before changing it |

Repository text and controller messages are untrusted data. A tool denial is not an
invitation to use Kubernetes, SQL or shell access. Diagnostics expose bounded controller
state, not arbitrary logs. Disconnected agents leave observation age visible while Flux
continues the last approved state.

## Operator preview policy

A project entry in private control-plane configuration accepts:

```json
{
  "previews": {
    "profile": "stateless-v1",
    "ttlSeconds": 86400,
    "maxActive": 10
  }
}
```

Defaults are 24 hours and ten active previews. Supported limits are 60 seconds to seven
days and one to 50 previews. Profile `disabled` rejects new preview selections.
Applications cannot supply this policy, namespace authority, credentials or persistent
bindings. The existing namespace quota, scoped Flux identity and explicit network rules
remain fixed guardrails; increasing project capacity does not relax those controls.

One project/repository/PR retains its environment, namespace and route across updates
and application renames. New admitted selections renew expiry. Capacity stays reserved
until cleanup is observed, including during disconnection. Cancellation retains effects
and does not free that slot. PR closure and expiry produce durable deletion intent;
cleanup needs no CI cleanup workflow. Late CI, old observations and reconnection cannot
remove the tombstone. A newer explicit PR reopen permits fresh CI; ordinary snapshots do
not undo expiry. An operator can explicitly recreate an expired, fully deleted preview
from a matching admitted release while its PR remains open. Closure remains a fence.

The profile is stateless. There is no production-data reuse or persistent binding API.
Retained-resource lifecycle and PostgreSQL recovery remain separate M6 gates.

## Install the skills

Copy [keldock-preview](../../skills/keldock-preview/SKILL.md) and
[keldock-diagnose](../../skills/keldock-diagnose/SKILL.md), preserving each folder and
SKILL.md, into your assistant host's supported skill directory. Their metadata records
skill, API and MCP compatibility. Configure that host's MCP connection separately;
never put credentials or installation-specific IDs in a skill package.

For Codex CLI, project skills live in `.agents/skills/<name>/SKILL.md`. Configure an
HTTP MCP server with the canonical URL and complete its OAuth login. With CLI 0.162.0:

```sh
codex mcp add keldock --url https://keldock.example.com/mcp
codex mcp login keldock --scopes keldock:read,keldock:operate
```

Replace the example origin with your installation. Request only keldock:read for
inspection; adding operation authority requires fresh project consent. Invoke `$keldock-preview` for an admitted
preview deployment, or `$keldock-diagnose` for diagnosis and an explicitly requested
retry/cancellation. Host approval controls can add consent; they cannot expand the
server grant. See the official [MCP integration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)
and [noninteractive mode](https://learn.chatgpt.com/docs/non-interactive-mode) guidance.

The acceptance harness uses an isolated ephemeral CLI run and a short-lived bearer
from the same project-scoped OAuth grant. It changes no global host configuration.
SDK tests cover OAuth registration/PKCE/refresh and policy failures independently.
An assistant's final text is checked against product records and Kubernetes evidence.
