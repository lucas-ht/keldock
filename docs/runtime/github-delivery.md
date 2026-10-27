# GitHub delivery profile

The experimental GitHub.com integration connects an installation-owned App, trusted
user CI and customer-controlled Kubernetes. Its self-hosted code requires no enterprise
GitHub attestation feature or paid Keldock account. General installation packaging is
still a separate gate. See [process configuration](processes.md).

## Connect the App

Set `githubCredentialsFile` to an installation-private JSON destination in a mode 0700
directory, then open **GitHub delivery** as an administrator. The destination may be
absent before registration. The manifest flow proposes a private, installation-owned
App; the operator reviews registration and selects repositories on GitHub. It does not
request user OAuth access. An existing App can be imported with its App/client IDs,
slug, RSA private-key PEM and webhook secret.

The supported repository permissions are Metadata, Contents and Actions read, plus
Checks, Deployments and Pull requests write. Events are pull_request, push, repository
and workflow_run; GitHub also delivers installation lifecycle events. Configure the
signed webhook to the installation's canonical HTTPS `/v1/github/webhooks` endpoint.
**Update webhook** uses the stored secret through the App API. An existing App must
have an active webhook; TLS verification remains enabled.

Setup states expire after ten minutes, are single-use, and bind the initiating human's
current authority and configuration version. Callback query parameters never grant
repository authority. Synchronization reads GitHub's actual selected-repository list,
stable repository/owner IDs and current permissions. Repository API access is rechecked
before CI admission and feedback; stale grants do not authorize a submission.

Renames change routing without changing the approved repository identity. Transfers to
an unapproved owner fail closed. Local installation disconnection survives synchronization;
App disconnection revokes new CI/feedback authority while retaining running workloads
and the upstream App. Rotation verifies the same App identity and atomically replaces
its private file. Current administrator authority and configuration versions gate these
mutations; App keys and webhook secrets are excluded from public status and audit text.

## Keep the CI integration thin

CI owns builds, tests and publication. After producing all component image digests,
run the preparation and signing steps in a trusted job:

```sh
keldock prepare --file keldock.yaml --images images.json --project sample --output unsigned.json
keldock attest --private-key build.pem --key-id build-key --input unsigned.json --output release.json
```

Preparation reads GitHub's source/workflow/run environment. PR releases distinguish the
tested merge commit from the source head. Signing uses the existing repository-scoped
DSSE/Ed25519 profile. Keep its private key unavailable to untrusted fork code; never
use pull_request_target to execute that code with privileged secrets.

Pin the Keldock Action to a reviewed full commit. Private development repositories can
use GitHub's owner-level private Action access; public publication is unnecessary.
The separate `.github/actions/setup` utility builds the pinned CLI using supplied Go.
It is CI tooling and is not part of the deployment Action's runtime.

```yaml
- uses: owner/keldock@REVIEWED_FULL_COMMIT
  with:
    api-url: https://keldock.example.com
    project: sample
    release-file: release.json
    wait: "true"
    timeout: "180"
```

The Action obtains an installation-audience Actions OIDC token, submits the signed
complete release, persists its durable receipt before polling, and optionally waits
for fresh selected-revision readiness. Exact workflow identity is required; an absent
job_workflow_ref or one equal to workflow_ref is supported. A different executing
workflow remains outside this direct-workflow profile.

Identical retries preserve request bytes and idempotency identity after an uncertain
acceptance response. Outputs include accepted/ready, all receipt IDs, generation, URLs,
state and wait-timed-out. `wait: "false"` returns after acceptance. A wait timeout or
terminated runner never cancels accepted work. Retry/promotion use admitted images.

## Separate registry authorities

GitHub App API access does not authorize private GHCR pulls. GitHub's supported
[GHCR authentication profile](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
uses GITHUB_TOKEN for CI publication and an external PAT classic for persistent pulls.
The reader needs read:packages plus actual package access; SSO authorization may also
be required by its owner. An Actions job token is not a durable cluster credential.

Keep three authorities separate: CI image publication, control-plane OCI bundle
publication, and cluster read access. If the control plane stores bundles in GHCR,
its publisher requires a separate write:packages credential. Neither publishing token
belongs in workload namespaces or CI deployment receipts.

Set the agent's `registryCredentialsFile` to an owner-only regular JSON file:

```json
{
  "bundle": {
    "repository": "ghcr.io/owner/bundles",
    "username": "registry-reader",
    "password": "READER_CREDENTIAL"
  },
  "workloads": {
    "sample": [
      {
        "repository": "ghcr.io/owner/application",
        "username": "registry-reader",
        "password": "READER_CREDENTIAL"
      }
    ]
  }
}
```

The registrar reloads this file, matches the approved project and exact repositories
actually used, and projects only required namespace pull Secrets. Flux's bundle source
uses its own same-namespace secretRef. Workload service accounts and trusted Flux patches
reference image pull Secrets; application OCI content contains none of these credentials.
Flux's workload reconciler has no Secret-read permission. Rotation updates owned Secrets;
removing a profile removes its references and Secret. Invalid files fail new reconciliation.

Kubelet retry and credential caching can delay recovery. Revoking a reader does not stop
running containers or erase cached images. Acceptance must use a cold immutable image
when proving denied access. This is not hostile-tenant isolation. The production profile
supports GHCR first; other registries retain their separate evidence gates.

## Durable events and feedback

Webhooks verify HMAC over exact bytes, durably deduplicate deliveries and fence access
removal before asynchronous processing. Leased handlers recover after interruption;
periodic authoritative repository/PR snapshots repair missed lifecycle events. Failed
work retries with bounded backoff and respects GitHub rate-limit delays. Exhausted
webhook work and failed reports are visible in the console and can be retried by an admin.

Feedback discovers accepted operations from the durable journal. It publishes App-owned
Checks, GitHub Deployments/status and one marked PR comment. Stored remote IDs, marker
recovery and per-PR leases recover partial effects and lost create responses. GitHub
does not provide a cross-system transaction or a unique external-ID constraint; these
are serialized, recoverable effects rather than an exactly-once API claim.

Acceptance, rollout progress, fresh readiness, stale observations, failure, supersession
and preview removal have distinct feedback. Old builds cannot deliberately overwrite
the current PR comment. Reporting outages do not cancel or admit deployments, and
feedback delivery resumes independently of the original runner.

Primary contracts: [App manifests](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest),
[App webhooks](https://docs.github.com/en/rest/apps/webhooks?apiVersion=2026-03-10),
[Checks](https://docs.github.com/en/rest/checks/runs?apiVersion=2026-03-10),
[Deployments](https://docs.github.com/en/rest/deployments/deployments?apiVersion=2026-03-10)
and [comments](https://docs.github.com/en/rest/issues/comments?apiVersion=2026-03-10).
