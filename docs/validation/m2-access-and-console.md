# M2 access, console and MCP evidence

Recorded 2026-10-27 on macOS/arm64, against the owned disposable keldock-m1 Docker
network, PostgreSQL database, registry and k3d cluster. Exact target/version pins are in
the [local runtime guide](../runtime/local-development.md). No cloud, production or
existing application installation was changed. This is an experimental development
profile. A subsequent operator scope decision accepts M2 and moves private GHCR pulls
to M3, with other registry profiles deferred. It adds no private-registry test evidence.

## Implementation and reproduction

The identity foundation is commit 09062ac; API/agent enforcement, shared inspection,
retry and MCP adapter are commit c5baf28, with SDK/revocation follow-up 045c4b2.
The console, design system, fixture and HTTP acceptance are commit 5dd048a.
Together these complete the requested M2 access/UI slice. See the
[authentication contract](../runtime/authentication.md), [console guide](../runtime/console.md)
and [ADR 0005](../decisions/0005-shared-authentication-and-remote-mcp.md).

```sh
mise exec -- task bootstrap
mise exec -- task doctor
mise exec -- task local:test
```

The full local suite passed 19 integration tests: ten runtime and nine access tests,
including real PostgreSQL, OCI distribution, Kubernetes, Flux and ingress. Runtime tests
took 518 seconds; access tests took nine seconds. Subsequently, targeted race-enabled
integration runs passed the standard SDK refresh/narrowed-project regression and three
new acceptance tests: bounded pending OAuth requests, compiled CLI account recovery and
enrollment, and revocation interrupting an in-flight agent driver. The current suite
therefore contains 22 integration tests; that count does not claim a second full run.
The final targeted run repeated all ten access tests and the three affected runtime
MCP/revocation/CLI tests successfully with race detection enabled.

## Proven local behavior

| Area                 | Evidence                                                                                                                                                                  |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Accounts             | Argon2id passwords, one-time invitation/activation, account/version/project authority, session expiry/revocation, last-admin protection, recovery and login limits        |
| HTTP authentication  | Real cookies, exact-Origin/session-bound CSRF, role/project denial and administrative account/cluster/MCP settings                                                        |
| Agent identity       | One-time enrollment/expiry, exclusive session lease, credential rotation and lost-response recovery, persisted revocation and heartbeat cancellation                      |
| Compiled CLI         | Bootstrap writes a private invitation, recovery revokes previous sessions, enrollment writes an owner-only credential, no secrets on stdout                               |
| Delivery and retry   | Real API/worker/agent/Flux lifecycle, complete component graph, API-only image update, failure/recovery, cancellation, new retry generation and immutable admitted inputs |
| MCP interoperability | Official Go SDK v1.8.0 discovery, dynamic registration, S256 PKCE, explicit consent, token exchange and standard SDK refresh                                              |
| MCP authority        | Read-only tools, configured-but-ungranted project denial, stale observations, bounded redaction, endpoint/client/account/grant revocation and replay protection           |

The agent interruption test uses a waiting driver and real HTTP/identity storage to
prove cancellation; it is separate from the real Kubernetes delivery tests. OIDC and
webhooks use local signed issuers/events, not a live GitHub App or workflow.

## Console verification

`task doctor` passed, including TypeScript and a production Next.js build. With both documented
fixture processes running, `task local:console:test` passes HTTP acceptance for the
same-origin proxy, HttpOnly/SameSite cookies, authenticated server rendering, no-store,
CSRF, viewer/project denial, logout and MCP metadata/authentication challenges.

Built-in-browser QA covered laptop/1440px desktop and 390px mobile viewports, light/dark
themes, environment filtering/selection, details, loading/empty/error states, people
forms, cluster confirmation, MCP settings and consent. Viewer navigation and direct
People access were denied. A controlled API stop/restart exposed an error-recovery bug;
the corrected Try again reload now returns to sign-in after the API is restored.
Credential changes and final access grants were tested through HTTP/SDK acceptance;
browser form/dialog inspection alone is not counted as proof of those mutations.

An independent Impeccable finish review found clipped mobile diagnostic values and a
hidden mobile fixture notice. Both were corrected, recaptured and scored resolved in
the reviewer's ship verdict. The review used the code-led Field Guide sketch and the
rendered application; separate catalog quality imagery was unavailable. Field Guide
was an explicitly stated unattended assumption after an unanswered direction preview,
not a recorded user approval. The detector ran once and returned no findings.

The fixture shows synthetic accounts and three real local application environments.
It verifies fixture provenance internally and labels the installation accordingly; it
does not demonstrate live GitHub admission. Raw screenshots and logs remain ignored
under .impeccable/review and .local; no secret or generated kubeconfig is committed.

## Remaining gates

- Private GHCR pull acceptance: moved to M3 by operator decision. Publisher credentials
  exist, but the current pull fixture uses an unauthenticated local registry. AWS/ECR,
  Docker Hub and private Harbor-style registry profiles follow later.
- A documented assistant-host integration and versioned skills, bounded MCP writes and
  nonproduction selection: M4; the SDK pass does not imply every assistant host works.
- Live GitHub App/Actions/reporting: M3; general installation, upgrade and control-plane
  backup/restore plus PostgreSQL capability recovery: M6.
- Cloud provisioning, hosted tenancy, minimum supported footprint, availability and
  hostile-tenant isolation retain separate evidence gates.
