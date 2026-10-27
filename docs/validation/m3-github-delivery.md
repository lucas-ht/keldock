# M3 GitHub delivery evidence

Recorded 2026-10-27–28 against an explicitly approved private test App/repository and
private GHCR packages. The control plane, agent, PostgreSQL, Flux and workloads ran on
the owned disposable keldock-m1 target on macOS/arm64. An operator-configured temporary
HTTPS tunnel exposed the development console/API to GitHub. No cloud or production
installation was changed. Private configuration, logs, credentials and detailed fixture
identifiers remain outside tracked product files.

## Implementation and reproduction

The implementation layers are App delegation/webhooks (1e78706), scoped registry
projection (70ead5b), the thin Action (b3e24b2), onboarding/durable feedback (d27ecd4),
actual direct-workflow OIDC compatibility (43f6f16), GitHub console administration
(c889d8a), stable repository rename authority (36f2dd4), and large comment-ID persistence
(f5ea678). Persistence failures are reported instead of silently leaving comment leases
held. See [the delivery guide](../runtime/github-delivery.md) and
[private process configuration](../runtime/processes.md).

```sh
mise exec -- task bootstrap
mise exec -- task doctor
mise exec -- task local:test
```

task doctor and the final task local:test passed on 2026-10-28. The full race-enabled
local suite passed 29 integration tests: 13 runtime (614 seconds), ten access (ten
seconds), and six GitHub tests (six seconds). These exercise real PostgreSQL, OCI distribution, Kubernetes,
Flux and ingress. GitHub contract tests use a controlled upstream; the live tests below
supply separate evidence for GitHub.com, Actions identity and GHCR. A large-ID regression
uses a remote comment ID above the signed 32-bit range and checks both persisted identity
and lease release.

Repository checks include formatting, Markdown/local links, repository and pin contracts,
11 JavaScript tests, Go formatting/vet/race tests/lint, Action workflow lint, TypeScript
and the console production build. A GitHub-hosted Linux/amd64 doctor/runtime run passed
for d27ecd4; that historical run does not claim validation of subsequent fixes.

## Live acceptance

| Boundary             | Observed result                                                                                                                                                                                                                                                                                            |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| App setup            | Imported the existing installation-owned App, configured a signed HTTPS webhook and installed only on the approved private fixture repository with the documented minimum permissions. Manifest registration/state fencing also passed controlled upstream tests; a second live App was not registered.    |
| Current authority    | Local disconnection denied submission and survived synchronization. Reconnection restored access. Actual upstream suspension denied access despite a cached active grant; resumption recovered it.                                                                                                         |
| Rename and transfer  | A real rename preserved stable repository/owner authority and current routing, including historical run names. Transfer to a different owner denied access. GitHub removed the selected-repository installation during transfer; after return, an explicitly approved scoped reinstall restored authority. |
| Credential lifecycle | Replaced the App key and reader PAT. The operator revoked the original credentials; the old App key received 401 and old reader 403, while current credentials succeeded. The reader has read:packages; publisher credentials are separate.                                                                |
| Thin Action          | User CI built/tested/published a multi-component Vite/API/worker/job/schedule image, prepared and signed a complete release, then submitted through actual audience-bound Actions OIDC. Keldock performed no image build or publication on CI's behalf.                                                    |
| Uncertain acceptance | A development-only fault closed the HTTP connection after the real admission transaction committed. The Action retried the identical request and obtained the one durable receipt rather than selecting a second generation.                                                                               |
| Stale workflow       | After a newer workflow had been accepted, rerunning the older workflow received HTTP 409. The newer staging generation remained selected.                                                                                                                                                                  |
| Private pulls        | Both private GHCR bundles and workload images reconciled. With bundle access retained and a revoked workload reader, a cold immutable image failed with a real GHCR 403. Restoring the valid reader made the same accepted operation ready without rerunning CI.                                           |
| Client timeout       | The Action's bounded wait expired while the release remained accepted. It did not cancel the operation; the recovered deployment later became ready.                                                                                                                                                       |
| Runner interruption  | Stopped a live Actions runner after durable admission. GitHub recorded cancellation; the same operation/generation reached Ready, and the existing single PR comment updated with the new head and tested merge SHA.                                                                                       |
| Preview closure      | Closed the disposable PR without merging. Signed close delivery drove durable deletion, removed the preview namespace and marked the Deployment inactive; the same PR comment changed to Preview removed. Staging remained ready.                                                                          |
| Reporting outage     | A development-only transport fault interrupted Checks/Deployments/comments while authoritative admission reads remained available. Workload recovery continued. The queue survived a control-plane restart and delivered feedback after recovery independently of the departed runner.                     |
| Commit identity      | The live PR Check targeted its source head; the GitHub Deployment recorded the distinct tested merge commit. Feedback displayed both.                                                                                                                                                                      |
| Delivery replay      | GitHub redelivered an actual earlier push with the same delivery GUID and received 202. Durable deduplication retained one event record and did not create a new deployment.                                                                                                                               |

The fixture uses synthetic HTTP side effects for preparation, worker heartbeat and
scheduled execution. Readiness covers all five selected components; it does not prove
an application database migration or data recovery. The local authenticated registry
uses fixture credentials and is not a substitute for the live GHCR permission test.

## Console and failure review

Built-in-browser review covered the GitHub page at desktop and 390px mobile widths,
light/dark themes, credential dialog, selected repositories and recovery state. Live
Sync, installation disconnection/reconnection and failed-report retry used the same
administrative APIs as other clients. Credentials were never prefilled into public
status, diagnostics or MCP responses. Actual browser rendering of the Kubernetes-hosted
Vite app confirmed frontend/API source identity and real preparation/worker/schedule
responses.

Live acceptance found and corrected direct-workflow job_workflow_ref handling, renamed
repository routing, and large remote comment-ID persistence. The fixture also gained
bounded startup request retries after exposing an initial dependency timing failure;
that failed rollout was not counted as readiness.

## Limits and remaining gates

M3 covers this GitHub.com/GHCR development profile. It does not establish a packaged
self-hosted alpha, production availability, hostile-tenant isolation, supported minimum
footprint, or cloud operation. General installation/upgrade and control-plane backup
recovery remain M6. AWS/ECR, Docker Hub and private Harbor-style registries require their
own credential refresh, TLS, access and lifecycle evidence.

App API access cannot replace GitHub's persistent GHCR reader credential. Rotation and
revocation do not evict running containers or cached images. Feedback uses bounded
marker recovery and leases; it does not promise exactly-once external API effects.
GitHub rate-limit parsing/backoff is covered by controlled tests, rather than deliberately
exhausting the live account's quota.

MCP remains the M2 read-only profile. Bounded writes, versioned skills and an actual
assistant-host workflow are M4. Optional capability installation, PostgreSQL recovery,
hosted tenancy and hosting keep their later evidence gates.
