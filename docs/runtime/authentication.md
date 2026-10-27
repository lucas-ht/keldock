# Accounts, agent identity and MCP access

The Go API is the authorization authority for the Next.js console, cluster agent and
MCP adapter. The self-hosted implementation needs no paid account or enterprise identity
provider. This replaces the earlier static operator and agent development credentials.

## Human accounts

An operator with the installation's private database configuration bootstraps the first
administrator. There is no public signup endpoint or default production password:

```sh
keldock bootstrap --config control.json --email operator@example.com --output invitation.json
```

The owner-only output contains a single-use activation URL, valid for 48 hours. Share
it privately; Keldock does not send an email. Invitations use a fragment so their secret
is absent from HTTP request URLs and referrers. Activation accepts a 12–256 character
password and creates a fresh session. Password hashes use salted Argon2id, 64 MiB,
three iterations and parallelism two, following the
[OWASP password guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html).

The console's People screens let administrators invite accounts, change their name,
role, status and project grants, disable accounts and replace activation/reset links.
Administrator is installation-wide; operator can read, retry and cancel within assigned
projects; viewer can read those projects. The last active administrator is protected.
Updates require the expected account version and revoke existing sessions and MCP grants.
Changing your own password replaces the current session and revokes other sessions/grants.

Opaque session credentials contain 256 random bits; only their SHA-256 verifier is stored.
They have a 12-hour absolute lifetime and 30-minute request-idle lifetime. Each request
checks the current account version, status and revocation. Production cookies use
`__Host-keldock_session`, Secure, HttpOnly, SameSite=Lax, Path=/ and no Domain. The
explicit loopback HTTP development profile uses `keldock_session`. Session mutations
require the exact configured public Origin and a session-bound HMAC CSRF proof. Login
has a durable per-account budget; concurrent password work is bounded. See the
[OWASP session guidance](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).

Operator recovery is available without the UI or a hosted account:

```sh
keldock recover --config control.json --email operator@example.com --password-file replacement-password
```

The password file must be owner-only. Recovery changes an active account's password
and revokes its sessions and MCP connections. It does not grant new project privileges.

## Cluster identity

Clusters are fixed by operator configuration. An administrator creates a ten-minute,
single-use enrollment grant in Clusters. The operator puts that token in an owner-only
file and supplies a private agent configuration with `apiUrl` and `credentialFile`:

```sh
keldock enroll --config agent.json --cluster configured-cluster --token-file enrollment.token
keldock agent --config agent.json
```

Exchange issues a credential restricted to one organization and cluster, valid for
30 days. Every RPC checks its verifier, expiry and cluster generation. A 30-second
exclusive lease prevents two processes from acting as the same agent. A ten-second
heartbeat cancels in-flight agent work on rejection or loss of connection; graceful
shutdown releases the lease. Flux retains the last approved workloads independently.

The agent renews credentials with less than seven days remaining. It atomically saves
a pending replacement before exchange; retrying that exact replacement recovers a lost
response without extending expiry twice. Private files are synced and replaced with
owner-only permissions. Replacing enrollment revokes the old identity only on exchange.
Revocation immediately rejects subsequent RPCs and cancels unused enrollment grants;
it does not delete workloads, namespaces or data. Re-enrollment is required to reconnect.

## Remote MCP

The endpoint defaults to disabled. Administrators enable it and manage registered public
clients from MCP access. Owners can inspect/revoke their own connections; administrators
can inspect/revoke installation connections. Disabling the endpoint revokes every grant
and pending request. Re-enabling it does not restore those credentials.

The adapter uses the [official Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
and stateless Streamable HTTP at the installation's canonical `/mcp` URL. It implements
protected-resource metadata, authorization-server discovery, public client registration,
S256 PKCE and exact registered callback/resource binding under the
[MCP authorization profile](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).
Callbacks require HTTPS or native loopback HTTP; no wildcard, userinfo or fragment is
accepted. Registration fetches no external callback or metadata URL. Client names are
unverified metadata, shown alongside the exact callback during consent.

The only scope is `keldock:read`. A signed-in person explicitly selects projects. An
administrator's MCP connection remains read-only and cannot inherit projects outside
that selection. Account changes, endpoint policy, client disable and grant revocation
are checked on every request and tool invocation; a browser cookie cannot authorize
MCP. Requests last five minutes, codes last 60 seconds, access tokens last one hour and
rotating refresh credentials share a maximum 30-day grant lifetime. Proven code or
refresh replay revokes the grant family. Refresh may omit `resource` under
[RFC 8707](https://www.rfc-editor.org/rfc/rfc8707.html#section-2.2); the stored grant still
fixes the only audience, and an explicit different audience is denied. Public client
registration and pending authorization requests are bounded.

| Tool              | Read contract                                                    |
| ----------------- | ---------------------------------------------------------------- |
| list_projects     | Projects allowed by the account and narrower connection          |
| list_environments | At most 25 current environment revisions, with a next cursor     |
| get_environment   | Current selected release, component topology, digests and health |
| get_operation     | One authorized operation and matching evidence/freshness         |
| get_diagnostics   | Bounded redacted controller evidence for the current generation  |

These tools share the console's inspection service. They expose no application environment
values, password material, kubeconfigs, agent credentials, arbitrary pod logs or shell
execution. Diagnostics retain freshness and distinguish missing evidence from healthy
workloads. Tool annotations describe read-only behavior; server policy enforces it.

Real SDK client acceptance covers discovery, registration, consent, exchange, standard
refresh, tools, narrowed project access, redaction, stale evidence and revocation. It
is not a compatibility claim for every assistant host. Hosted identity, SSO and MCP writes
remain separate later work; first-party skills follow working bounded tools in M4.
