# Validation evidence

## Bootstrap baseline

The Keldock checkout was initialized from its existing license and one-line README on
2026-10-27. Previous project prototypes, schema checks and runtime reports are not
present and do not count as current evidence. M0 is repository tooling and design only.

Local repository validation on 2026-10-27:

| Check                                              | Result and scope                                                                                                                       |
| -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Locked bootstrap                                   | Passed with the selected tool versions and dependency scripts disabled                                                                 |
| task doctor                                        | Passed: formatting, Markdown syntax/local links, identity/pin/lock/CI contracts, workflow lint and whitespace                          |
| Validator tests                                    | Six passed: relative/reference links, anchors, missing targets/images, path boundaries, external-link exclusion and malformed encoding |
| GitHub-hosted CI                                   | Configured; no hosted run claimed by this local bootstrap                                                                              |
| Product schemas/runtime/integration/cloud/recovery | Not implemented or validated                                                                                                           |

At M0, Node ran repository checks only; no product deployment, MCP server, agent skill
package, chart, runtime acceptance command or supported dependency matrix existed.

This table records M0 only. Current M1 code and local runtime evidence are described in
the [runtime guide](../runtime/local-development.md); it supersedes the bootstrap's
absence of a runtime acceptance command. The experimental core M1 is implemented.
Later [M2 evidence](m2-access-and-console.md) and [M3 GitHub delivery evidence](m3-github-delivery.md)
record subsequent implementation. [M4 evidence](m4-previews-and-agents.md) adds stateless
preview policy, bounded writes and actual assistant-host/skill workflows. Cloud and
persistent-data recovery remain separate gates.

## Core M1 acceptance

Local validation on 2026-10-27 uses Docker Desktop on macOS/arm64 and only the
keldock-m1 PostgreSQL/registry/cluster target. Versions, exact targets and reproduction
commands are in the [runtime guide](../runtime/local-development.md). The runtime
implementation is recorded in commits 9d9fb0d, 48e04c6 and 174d721.

| Check                                                                                                         | Result and scope                                                                                                                         |
| ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| task doctor                                                                                                   | Passed: repository checks, Go formatting/vet/race tests/lint, and workflow lint                                                          |
| task local:test                                                                                               | Passed: eight acceptance tests against real PostgreSQL, OCI distribution, Kubernetes, Flux and ingress                                   |
| [API/agent preview lifecycle](../../test/runtime/agent_test.go)                                               | Signed event/OIDC/provenance fixtures, full graph, API-only image replacement, job failure/recovery, two previews, closure and teardown  |
| [API and compiled processes](../../test/runtime/api_test.go)                                                  | Credential separation, scope checks, receipts, pagination, observation ordering; compiled serve/agent/inspect/attest commands            |
| [Flux contract](../../test/runtime/delivery_test.go)                                                          | Selected-revision health/dependencies, failed readiness/recovery, scoped RBAC, ownership denial, pruning, rollback and network isolation |
| [Durable state](../../test/runtime/postgres_test.go) and [worker recovery](../../test/runtime/worker_test.go) | Duplicate acceptance, lease expiry/restart, stale results, cancellation, close/replay ordering and recovery after actual OCI publication |
| GitHub-hosted checks                                                                                          | Configured to run doctor and local:test; no hosted execution claimed                                                                     |

The development fixture retains explicit disk, inode and memory eviction reserves and
waits for pressure conditions to clear. Capacity-related failed runs were diagnosed
as such, rather than counted as product passes. Raw logs, configuration and credentials
remain ignored in .local. Successful acceptance removes its application namespaces;
the registry and PostgreSQL volumes remain available for reuse.

The [runtime profile decision](../decisions/0004-first-runtime-profile.md) records the
remaining human-auth, enrollment, UI, registry, MCP and live-GitHub gates. This evidence
does not establish cloud behavior, production availability, data backup/restore, hostile
tenant isolation, or a supported minimum footprint. Cron readiness proves schedule
configuration; the job fixture's HTTP counter is not a database migration/restore test.

The later [M2 access and console evidence](m2-access-and-console.md) adds authentication,
enrollment/revocation, the console and real SDK MCP reads. The M1 record above remains
historical; its listed auth/UI gaps are superseded only by that specific local evidence.

## Reproduce repository checks

```sh
mise install
mise exec -- task bootstrap
mise exec -- task doctor
```

`task test` now runs validation-tool and race-enabled Go unit tests. `task doctor` also validates untracked Markdown
files that are not ignored. External URLs are not fetched by the link checker; primary
source research is a separate activity. `task local:test` exercises the dedicated
keldock-m1 runtime target.

## Recording future evidence

Record the commit, exact versions, target/profile, commands, outcomes, failure/recovery
cases and limits for each milestone. Distinguish:

- Static checks and contract conformance.
- Disposable local Kubernetes behavior through the real API/agent/controllers.
- Live GitHub and MCP-client integration.
- Approved cloud-provider behavior and negative access checks.
- Backup, isolated restore, upgrade and ownership handover.

Do not infer cloud behavior from mocks, deployment readiness from accepted requests,
recoverability from backup status, or tenant isolation from namespace creation. Evidence
must match the current implementation; revalidate affected claims after lifecycle changes.
Keep secrets, real customer identifiers and raw runtime logs out of Git.
