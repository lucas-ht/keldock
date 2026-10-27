# Keldock console

The first console uses pinned Next.js 16.4.3, React 19.3.1, Tailwind CSS 4.3.3,
shadcn/ui source generated with CLI 4.23.0, and Base UI 1.9.0. It prioritizes deployment
and environment health. The [account and MCP model](authentication.md) remains enforced
by the Go API, including when the console is bypassed.

The extracted [design system](../../apps/console/DESIGN.md) and its
[component sidecar](../../apps/console/.impeccable/design.json) record the implemented
light/dark tokens, responsive rules and reusable primitives.

## Run the disposable development profile

Install pinned tools and dependencies, then use two terminals from the repository root:

```sh
task bootstrap
task local:console:api
```

```sh
task local:console
```

Open `http://127.0.0.1:3800`. The API/worker/agent harness listens on loopback port 3801.
It refuses targets outside the owned keldock-m1 PostgreSQL, registry and cluster. The
owner-only `.local/console-account.json` contains synthetic fixture login credentials;
these are not production defaults. The UI labels this installation as a synthetic local
fixture. The harness seeds a five-component application with two healthy environments,
one failing prerequisite, and an empty project. These are real PostgreSQL/Flux/Kubernetes
states, not hardcoded frontend status values. Initial reconciliation takes time.

The seed verifies its local provenance but is an internal fixture trust root. Live
GitHub admission is not demonstrated by this seed; the runtime suite separately exercises
OIDC/provenance and signed webhook fixtures. This development command combines processes
for convenience and is not an installation or production packaging profile.

Stopping the processes retains approved workloads and PostgreSQL/registry volumes.
The next run reconnects using replacement enrollment. Use `task local:reset` only when
intentionally recreating the owned disposable cluster; no cloud or application deployment
is authorized by this fixture.

## Screens and behavior

Overview filters and selects environments while showing the selected generation's
component strip. It distinguishes a failed prerequisite, blocked dependents and a
configured schedule; configuration is not proof a scheduled execution completed. Stale
or missing observations never establish current health. Visible health screens refresh
every ten seconds, with manual refresh and loading/empty/error states.

Project/environment details show immutable digests/source identity, exact-generation
evidence, redacted controller messages and the last 50 durable selection events. Human
operators can cancel or retry an admitted release within their project. Retry creates a
new generation/deployment and keeps the original release/CI inputs; its idempotency key
survives a lost response. Cancel retains completed effects and does not imply rollback.

People manages invitations, roles, status and project grants. Clusters manages enrollment
and revocation. MCP access manages the endpoint, registered clients and connections; a
separate OAuth consent screen requires an explicit project selection. Account changes
and password recovery remain available in the FOSS core. Forms show failures and retain
safe input; secrets appear only after their explicit one-time creation action.

GitHub delivery supports manifest/existing-App setup, credential rotation, selected
repository inventory, local disconnection, synchronization and failed-delivery recovery.
Its administrator actions use the same session, CSRF and current-authority checks.
Private GHCR reader configuration is a separate operator-owned agent profile; the App
cannot replace that registry credential. See [GitHub delivery](github-delivery.md).

The console uses asynchronous request cookies and uncached API reads, following the
[Next.js authentication guidance](https://nextjs.org/docs/app/guides/authentication).
Server rendering forwards only the session cookie to the configured internal API origin.
Browser mutations use same-origin proxy routes and a fresh CSRF proof. No session or
agent credential is stored in JavaScript storage. Authenticated pages use no-store;
React cache only deduplicates session reads within a render request.

## Run behind your HTTPS endpoint

Build with `pnpm --filter @keldock/console build` and run the Next server separately from
the API/worker and agent. Configure server-only `KELDOCK_API_ORIGIN` for the internal Go
origin and set the Go installation's `publicUrl` to the canonical HTTPS console origin.
The proxy forwards `/v1/*`, `/oauth/*`, `/.well-known/*` and the exact `/mcp` endpoint.
Preserve the external Origin and Set-Cookie headers. Keep the API's database authority
and the agent's Kubernetes/registry identity outside browser configuration.

`task doctor` includes the console's TypeScript check and production build.
`task local:console:test` checks the running disposable proxy, cookies, authenticated
server rendering, no-store, CSRF, role/project denial and MCP metadata/challenges.
Rendered desktop/mobile and interaction QA is recorded separately from HTTP acceptance.
