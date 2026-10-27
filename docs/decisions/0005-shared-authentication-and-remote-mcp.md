# ADR 0005: Shared authentication and remote MCP

Status: Accepted for the experimental M2 profile.

This supersedes ADR 0004's static development operator/agent credentials and its initial
stdio MCP transport target. Its Go/PostgreSQL/OCI/Flux runtime and packaging boundaries
remain in force.

## Decision

Keep durable account, session, agent and OAuth authority in the Go API. Next.js serves
the console through same-origin proxy routes; MCP wraps the same scoped inspection
service. Local administrator login and recovery work without an enterprise identity
provider or paid service. Human sessions, enrolled agents and MCP grants are distinct
credentials; none can substitute for another.

Use remote Streamable HTTP MCP with the official pinned Go SDK and the MCP OAuth profile,
including S256 PKCE, explicit project consent, resource binding and durable revocation.
A manually copied universal bearer token would bypass the intended consent and lifecycle
model. The first adapter exposes five bounded read tools and no mutations. Standard SDK
refresh is tested without requiring an explicit resource parameter on refresh; the
stored grant always retains its exact resource and project subset.

The [authentication contract](../runtime/authentication.md) records lifetimes, roles,
CSRF, enrollment, rotation and recovery. The [console profile](../runtime/console.md)
records runtime boundaries and developer commands. Each interface authenticates at its
server data/mutation boundary; navigation visibility and tool annotations are not policy.

## Consequences and limits

The reusable self-hosted product retains its own authorization and recovery. Future
hosted control-plane identity can integrate with that boundary without requiring cloud
cluster-admin credentials in the public frontend or making enterprise SSO mandatory.
A later external identity integration must preserve current project grants and revocation.

The bounded built-in OAuth authorization service is additional security-sensitive code.
Real PostgreSQL, HTTP and SDK tests cover replay, expiry, current account authority,
redirect/resource binding, consent, refresh and denial. This evidence is local and does
not establish compatibility with every assistant host or production hardening.

MCP writes and versioned agent skills remain M4. Private registry pull profiles, installer
packaging, backup/restore and live GitHub integration retain their own evidence gates.
The console does not imply installed Vault, Temporal, Crossplane or database capabilities.
