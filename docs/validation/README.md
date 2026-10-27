# Validation evidence

## Current baseline

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

Node currently runs repository checks only. There is no product deployment, MCP server,
agent skill package, chart, runtime acceptance command or supported dependency matrix.

## Reproduce repository checks

```sh
mise install
mise exec -- task bootstrap
mise exec -- task doctor
```

`task test` runs validation-tool tests. `task doctor` also validates untracked Markdown
files that are not ignored. External URLs are not fetched by the link checker; primary
source research is a separate activity. `task local:test` will be introduced only with
the first real runtime fixture.

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
