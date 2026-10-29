# M6 self-hosted alpha candidate

Status: local developer acceptance passed, 2026-10-29. The independent human
walkthrough, operational public security-reporting route and publication approval
remain separate gates. No public release, production deployment or M6 cloud test is
claimed. M5's approved AWS fixtures remain removed.

## Targets and profile

- Docker Desktop on macOS/arm64, with the existing disposable `keldock-m1`
  PostgreSQL and fixture registries; no real customer data.
- Separate owned `k3d-keldock-m6` for native CNPG/backup acceptance, API port 26443,
  application ingress 28080. Each acceptance run creates an independent synthetic
  control-plane database and organization.
- Separate owned `k3d-keldock-m6-core`, API port 36443, ingress 38080, for core
  packaging. Its node is constrained to 2 CPUs and 4 GiB RAM with no extra swap.
  It contains Flux and Traefik, without CNPG, Barman or Crossplane CRDs.
- Fixture-only HTTPS gateways, private test CAs and supplied S3-compatible storage.
  The application/API validate TLS; no global trust or browser-warning bypass was
  used. Accounts, identities, kubeconfigs and logs stay in ignored `.local/m6`.

The [supported profile](../runtime/alpha/support.md) records exact versions. Images
have immutable digests. Upstream controller manifests use checked source hashes;
Barman's base64-encoded injected sidecar and Flux controller images are also pinned.
Host recovery used PostgreSQL 18.6 clients against server 18.4; the runtime image
contains its matching PostgreSQL 18.4 clients.

## Implemented checks and observed results

| Path                      | Evidence and limits                                                                                                                                                                                                                                                                                                           |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Static repository         | `task doctor` passed: formatting/links, repository/CI contracts, Go vet/race/lint, chart profiles and console typecheck/production build. Final committed-source run is recorded with the kit.                                                                                                                                |
| Core packaging            | HTTPS local administrator activation, session/CSRF, explicit enrollment and connected setup inventory passed. The actual packaged API/worker/registrar delivered web, API, worker, schedule and migration from supplied immutable synthetic-CI images.                                                                        |
| Restart/upgrade           | A replacement runtime image reused private credential/state PVCs and reconnected. Kubelet volume group permissions are repaired by the private init process; strict credential readers still reject public/group-writable inputs.                                                                                             |
| Revocation and uninstall  | Revoked registrar bearer was denied with 403 while the application's approved response stayed healthy. Helm uninstall preserved the exact UIDs of all four PVCs and the application namespace/workloads.                                                                                                                      |
| Supplied dependencies     | Reinstallation configured supplied PostgreSQL and registry endpoints, reusing the retained data under separately owned fixture resources. The new Helm manifest contains no dependency StatefulSets. Existing account/release data survived; explicit new registrar enrollment connected.                                     |
| Native application data   | Real PostgreSQL-backed web/API/worker/schedule/migration ran with a non-superuser application owner. Unbound web and cross-environment access were denied. Preview started empty; closure removed workloads and retained its database.                                                                                        |
| Native backup and restore | Prefix-scoped supplied writer and read-only restore identities passed allowed/denied probes. A real base backup completed, archive WAL was observed, and an isolated cluster replayed a marker committed after that base. No source replacement or application cutover occurred.                                              |
| Control-plane recovery    | Full native flow passed in 415.30 seconds: real exported-snapshot `pg_dump`/`pg_restore`, outage/reconnect, revoked recovered sessions/agents, stale-generation fencing, OCI export/import into the separate initially empty registry, and identical-image application upgrade with retained data.                            |
| MCP                       | Official Go SDK discovered ten tools. A populated retained allocation exposed safe backup/WAL/freshness evidence without private namespace/source/archive references. Cross-project read and revoked access were denied. Database mutation remains outside MCP authority.                                                     |
| Browser                   | Resource inventory and Setup were reviewed in the built-in browser. Desktop light/dark, 390-pixel responsive layout, stale evidence, and opening/canceling a lifecycle dialog were checked. CSS viewport and scroll width were both 390 pixels. Review used a local developer preview; packaged HTTPS was checked separately. |
| Regression acceptance     | Complete `task local:test` passed after the fixes: runtime 658.162 seconds, access 15.019 seconds, GitHub 9.535 seconds and cloud-contract fixtures 7.054 seconds. The original M3 application remained healthy at its original commit. These cloud tests are local contracts, not new AWS validation.                        |
| Release/publication       | Local binaries, chart, dependency manifests, source archive, skills, image manifest and checksums are prepared by `task alpha:package` from clean committed source. Exact image/source identities and materialization checks are recorded with the generated kit; human walkthrough and publication remain gated.             |

## Measured core footprint

The complete constrained core node was sampled four times over 30 seconds with the
five-component application active. Maximum sampled memory was **1,756 MiB**; cgroup
lifetime peak was **2,200 MiB**. CPU was constrained to two cores throughout.
These measurements include Kubernetes/controllers, the packaged public services,
registrar, control-plane PostgreSQL, bundle registry and synthetic application.
They establish a tested **2 CPU / 4 GiB development floor**, not production throughput,
HA, optional database-profile sizing or a mathematically minimal configuration.

Persisted volumes and restore copies require separate capacity planning. Kubelet's
disk reserve remained enabled. During repeated fixtures, Docker disk contention and
build/test overlap caused failed runs; unused build cache was removed under the
existing scoped permission, and older owned M6 fixtures were reversibly suspended.
No database/PVC was deleted to manufacture a passing result.

## Failures that changed the implementation

- The embedded bundle registry originally used a short Service name, which Flux in
  another namespace could not resolve. Generated aliases now use the full namespace
  Service DNS address; API publication and Flux pulls were exercised together.
- Retained enrollment files acquired group permission bits on remount. Init repairs
  only its bounded regular files in the private state directory; it rejects links
  and nested state. The chart also uses `OnRootMismatch` volume ownership handling.
- A revoked process entered a crash loop that prevented normal explicit enrollment.
  Packaged waiting mode now stops reconciliation and waits for an operator-written
  identity replacement, without retrying a rejected identity.
- Restore evidence applied product-name rules to an opaque Kubernetes UID. A real
  numeric-leading UID exposed the error. UID validation now accepts bounded opaque
  identifiers while excluding credential/path-shaped values, with a regression test.
- Preview expiry used a process clock against database enforcement. It now derives
  deadlines from the database clock. Flux retries also temporarily lose Ready=False;
  failure inspection uses newest matching completed history for the exact revision
  and observed specification generation, with stale/success/other-revision negatives.
- A full regression run also exposed a selected migration Job failure masked by
  Flux retry state. Observation now checks terminal `Failed=True` directly on
  the exact selected Job, deployment/environment and immutable image. Failed pod
  counts and other deployments/images cannot satisfy it. The full PR lifecycle
  then passed in 209.40 seconds and in the complete passing regression suite.
- A recovery rerun lacked `pg_dump` on PATH and failed after data restore had passed.
  The native test now checks both PostgreSQL 18 clients before creating resources.

The selected Job rule follows Kubernetes' documented
[terminal Job conditions](https://kubernetes.io/docs/concepts/workloads/controllers/job/#terminal-job-conditions).

## Reproduction and remaining gates

Use pinned tools, an adequately sized Docker VM and PostgreSQL 18 clients on PATH:

```sh
mise exec -- task doctor
mise exec -- task local:test
mise exec -- task local:alpha:postgresql:test
```

Native acceptance is separate from the default local test because it explicitly
installs optional controllers, creates retained data and requires supplied archive
fixtures. It uses synthetic local admission to test orchestration; it is not a new
live GitHub OIDC/attestation proof. M3 retains the existing live delivery evidence.

Core packaging uses `task local:alpha:core:up`, the reviewed candidate runtime/console
images and `tooling/alpha/install.mjs --core`. After synthetic packaged delivery and
measurement, `task local:alpha:core:lifecycle` runs the scoped retention/reinstallation
journey. The [operator guide](../runtime/alpha/installation.md) describes ordinary
installation without fixture-specific domains or infrastructure.

After developer acceptance, materialize a clean-source kit with `task alpha:package`
and explicit immutable image references. Verify its `SHA256SUMS` and CLI version.
A separate human must then complete the [walkthrough](../runtime/alpha/walkthrough.md).
Publish only after explicit review; leave M6 in progress until its actual exit gates
are satisfied.
