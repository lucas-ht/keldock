# M4 preview and agent acceptance

Date: 2026-10-28. Status: accepted for the stateless development profile.
This record distinguishes local contracts, an actual assistant host and live GitHub
from general installation, production availability and cloud validation.

## Local target and commands

Only Docker Desktop desktop-linux, cluster/context keldock-m1 / k3d-keldock-m1,
the owned PostgreSQL/OCI fixtures and synthetic application namespaces were used.
Private installation configuration, credentials, logs and captures remain ignored.
The reusable implementation starts with commits 5617d0c, df00cbb, 0605588, 6e292ee,
8c5808f and 8ad5132.

| Check                     | Result and scope                                                                                                                               |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| task doctor               | Passed: formatting, documentation/links, repository contracts, Go vet/race tests/lint, workflow lint, TypeScript and production console build  |
| task local:test           | Passed: 17 runtime, 12 access and six GitHub integration cases; real PostgreSQL, OCI, Kubernetes and Flux                                      |
| Runtime package           | 604.562 seconds, including selected-revision health, network/RBAC isolation, private registry rotation, interrupted agents and preview cleanup |
| Access/GitHub packages    | 10.840 / 5.540 seconds; explicit operation consent, refresh/revocation and current delegation                                                  |
| Portable skill validation | Both SKILL.md files passed the skill-creator format validator; format alone is not behavioral acceptance                                       |

The first full run failed while testing a new preparation Job during Flux recovery.
The focused case then passed in 223.97 seconds, and the complete suite passed, including
that case in 223.75 seconds. After the lock-order fix, another complete suite passed,
including that case in 228.25 seconds. Job/event diagnostics were improved; health checks were
not weakened. The original transient failure's cause was not established.

## SDK and policy evidence

[TestMCPBoundedWritesReceiptsSessionLossAndPolicyParity](../../test/runtime/mcp_writes_test.go)
uses the actual OAuth-enabled SDK and Streamable HTTP transport. It covers release
selection, identical receipt recovery after session closure, expected-generation denial,
changed-input rejection, cross-project denial, cancel/retry, production denial through
both MCP and HTTP, the actor operation budget and revoked cached authority.

[Access consent tests](../../test/access/mcp_operations_test.go) cover explicit operation
scope, viewer denial, narrowed projects, refresh scope preservation and transaction-time
revocation. The six read tools expose bounded controller data, never arbitrary logs,
application environment values, credential files or execution tools.

[Revocation lock-order tests](../../test/access/mcp_locking_test.go) hold real endpoint
or client revocation locks while another user's MCP mutation authenticates. The test
failed on the original joined locking query because it held the grant first. Ordered
account/endpoint/client/grant locking passes without the reverse dependency. Current
role, scope and revocation fences still run inside the desired-state transaction.

[Preview policy tests](../../test/runtime/preview_policy_test.go) cover stable identity
through application rename, bounded capacity including disconnected cleanup, serialized
competing admission, disabled profiles, source-head and foreign-repository fencing,
expiry tombstones and explicit recreation only after cleanup. Existing runtime tests
exercise actual namespace/RBAC/network guardrails and revision-bound observations.

## Real assistant host

Codex CLI 0.162.0 ran with existing account authentication, an isolated workspace,
project skill folders and only the local Keldock MCP server. The ephemeral JSONL run
ignored global user configuration/rules and kept its shell read-only. A short-lived
bearer from the example project's explicit OAuth operation grant supplied MCP access;
OAuth registration/PKCE/refresh are separately covered by the SDK tests. No API key,
paid Keldock service or embedded inference provider was introduced.

Both version 1.0.0 skills were copied unchanged from the tracked skill folders. The
host read SKILL.md successfully, then invoked actual Keldock tools. Its shell inherited
only a minimal executable path, without the MCP bearer environment variable. The first
selection run had an overly restricted shell path and could not read the skill; that
run proves the tool call, not skill execution. A corrected second session read the
preview skill and recovered the identical original receipt without selecting again.

The preview workflow selected the CI-admitted PR release at generation 1, yielding a
generation 2 receipt. The submitting assistant exited while publication was pending.
A second session replayed the original five arguments/key, recovered the same IDs and
inspected fresh matching Ready state. Product and Kubernetes records were checked
independently; assistant prose was not accepted as the source of truth.

The diagnosis skill inspected a genuine failed rollout at generation 3. After the
operator restored the reader configuration, the agent remained disconnected while the
host requested retry. The returned generation 4 reused the same admitted release and
five image digests. The assistant reported missing/stale observation and did not claim
that the credential repair or rollout had succeeded. Agent reconnection then reached fresh matching Ready state at generation 4, with no
changed release or image input. The browser confirmed matching updated frontend/API
commits and an active worker after the submitting session ended.

Reproduce a host acceptance run with the installed skills and MCP connection using the
[workflow guide](../runtime/agent-workflows.md). The harness used `codex -a never -s
read-only -C <isolated-workspace> exec --ignore-user-config --ignore-rules --ephemeral
--json --skip-git-repo-check`, MCP URL/bearer-environment overrides and a prompt naming
the skill, exact nonproduction target and stable request key. Captures stay private.
These results do not claim compatibility with every assistant host.

## Live application and rendered console

The disposable private [keldock-example PR #2](https://github.com/lucas-ht/keldock-example/pull/2) used user-owned CI to publish a private
multi-platform image, prepare/sign the complete release and submit through the existing
thin Action. Initial workflow 40644847751 succeeded. All five components became ready;
the browser showed matching frontend/API tested merge commits, preparation count two,
a current worker heartbeat and a completed scheduled run.

A second commit updated the same PR/environment/namespace/route. Selecting the old
release after the source-head change returned structured stale_revision. Workflow
40645547509 successfully built and admitted its new release, then correctly failed its
readiness wait after deliberately invalid workload-reader credentials caused an actual
ImagePullBackOff. The independent bundle reader remained valid, and the last approved
application stayed running. The reader was restored through the agent-owned private
profile; assistant tools did not receive or administer it.

Browser checks covered operation consent wording and disabled approval without project
selection, release identity review, a disabled empty catalog after the PR-head change,
and a console stale-generation error after MCP changed the selection. Mobile checks
verified an actual 390 by 844 viewport with no horizontal overflow and light/dark
rendering. Browser emulation is temporary and restored afterward. Paging is covered by
the shared API contract; this live catalog did not require another page.

## Live lifecycle outcomes

| Generation | Observed behavior                                                                                                                                                                               |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1          | CI-admitted initial PR preview; all components ready                                                                                                                                            |
| 2          | Assistant selection; session ended during publication, identical receipt recovered in a second session, then Ready                                                                              |
| 3          | PR update reused the environment/route; genuine private image-pull failure, blocked dependencies, previous workloads retained                                                                   |
| 4          | Diagnosis skill requested unchanged-release retry while the agent was offline; reconnection reached Ready                                                                                       |
| 5          | SDK selected while agent was offline, then canceled the exact queued operation; running frontend/API remained healthy                                                                           |
| 6          | Retry of the canceled operation reused admitted inputs and reached Ready                                                                                                                        |
| 7          | Explicit 60-second selection accepted while the agent was disconnected                                                                                                                          |
| 8          | Durable expiry/delete intent; namespace initially remained. Identical generation-7 receipt replay did not resurrect it. Agent reconnection removed namespace and routes                         |
| 9          | After cleanup and late CI rejection, both current skills ran in a fresh host session and explicitly recreated the same environment/route from the admitted release; independently reached Ready |
| 10         | PR closed without merge; signed server-side lifecycle cleanup removed the namespace, workload route and active deployment                                                                       |

The real CI rerun of workflow 40645547509, attempt 2, built and signed its artifacts,
then submission was rejected with HTTP 409 after expiry. The target stayed deleted at
generation 8; the rejected attempt was absent from the admitted catalog. Recreation
was a separate explicit intent, not an automatic response to CI or reconnect.

The normal operator policy was restored before recreation. After closing PR #2,
selection with a fresh key/current generation was denied as stale_revision. The
namespace was absent and its explicit ingress Host route returned 404; staging still
returned 200 and retained its original release at generation 4 with fresh Ready health.
Cleanup required no CI cleanup workflow. The same single GitHub App comment was updated
to Preview removed, and the last deployment status became inactive.

The fixture PR is closed without merging; the test branch retains its source evidence.
The original staging application remains available. Temporary invalid pull configuration
and the short-lifetime policy were restored. The acceptance-only MCP grant was revoked
after the final host run; its old credential was denied, with other connections unchanged. The final assistant captures contained no
matches for the fixture's credential values. A separate operator diagnostic accidentally
printed the test reader token into the private task transcript; it should be rotated
before reusing that credential. Product tools did not emit it and it was not committed.

## Limits

The stateless profile does not implement persistent-data previews or database recovery.
Retained-resource APIs, optional PostgreSQL and external-operator installation remain M6.
No AWS/ECR, Docker Hub, Harbor, hosted tenancy, production promotion, cloud mutations,
backup/restore, hostile-tenant isolation or minimum production footprint is claimed.
The test workload's counters are in memory and are not a database migration test.
Application routes were checked through the local ingress with explicit Host headers;
public preview DNS/TLS was not validated. A temporary HTTPS tunnel served the control
plane for GitHub, and a tunnel/network interruption was not counted as a product pass.
