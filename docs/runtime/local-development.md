# Disposable runtime validation

Status: experimental core M1 runtime with a working API, worker and outbound agent.
The Action, MCP endpoint and general installation remain later gates; this is not an alpha.
See [process commands and configuration](processes.md) and the
[profile decision](../decisions/0004-first-runtime-profile.md).

## Prepare and run

With Docker running and the pinned mise tools installed:

```sh
mise exec -- task bootstrap
mise exec -- task doctor
mise exec -- task local:test
```

`local:test` prepares the dedicated environment, builds/publishes a synthetic CI image,
and runs real PostgreSQL and Kubernetes tests. The test harness builds that fixture;
Keldock's compiler and worker consume prebuilt image digests and do not run builds.

| Target                         | Exact local scope                                      |
| ------------------------------ | ------------------------------------------------------ |
| Docker Compose project/network | keldock-m1                                             |
| Cluster/context                | keldock-m1 / k3d-keldock-m1                            |
| Kubernetes API                 | 127.0.0.1:16443                                        |
| HTTP ingress                   | 127.0.0.1:18080 with an explicit test Host header      |
| Registry                       | 127.0.0.1:5010; cluster alias keldock-m1-registry:5000 |
| PostgreSQL                     | 127.0.0.1:55432, database keldock                      |

There is no cloud account or Terraform backend. The bootstrap refuses unrelated
same-named networks/clusters and uses .local/kubeconfig without changing the default
context. Secrets/configuration in .local are ignored by Git and created with restricted
file permissions. Do not copy them into an application repository.

The local profile explicitly installs Flux source/kustomize controllers and uses the
K3s-provided Traefik profile. Versions/images are pinned in mise and local manifests.
Flux rejects remote bases, cross-namespace references and missing scoped identities.
The local dependency retry interval is two seconds; regular reconciliation is one minute.
The development kubelet keeps explicit 10% free disk and 5% free inode reserves on
Docker's shared filesystem and retains the 100 MiB memory reserve. All four disk/inode
eviction signals remain configured explicitly. This drop-in
also sets a 1% minimum reclaim buffer: the inherited K3s 10% buffer otherwise requires
20% free after pressure starts. Bootstrap waits for node readiness and pressure recovery
before attempting rollouts. These settings are specific to the disposable profile;
see [Kubernetes eviction settings](https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/)
and [K3s configuration drop-ins](https://docs.k3s.io/installation/configuration).

When cluster configuration changes, `task local:reset` recreates only this owned cluster.
The registry and PostgreSQL volumes remain outside that reset. Bootstrap detects a
configuration mismatch instead of silently reusing an incompatible cluster.

## Current evidence

The disposable tests exercise:

- Concurrent duplicate acceptance returning one durable receipt, cross-project denial,
  uncertain retry conflict, lease expiry and stale worker completion.
- Worker interruption after an actual OCI publication but before database completion,
  followed by recovery with exactly the same content-addressed bundle.
- Durable preview close intent, duplicate webhook delivery and refusal of late CI or
  older open events after closure.
- A real API/web/worker/release-job/schedule graph with bounded resources and probes,
  release-job effects, service connectivity and a working ingress route.
- Scoped Flux impersonation, denied namespace/RBAC/secret/registration access,
  stale-generation refusal, failed readiness and recovery, pruning and rollback.
- Two environments with successful permitted traffic and denied cross-preview traffic.
- The API-to-agent preview lifecycle, real API-only image replacement while keeping
  the web pod, failed prerequisite jobs and recovery, signed close events and teardown.
- Compiled serve/agent processes, private configuration, CI attestation, supplied
  dependency inspection, publication and complete five-component health through HTTP.
- API credential separation, project denial, complete pagination, and rejection of
  old observations. Agent unit tests cover revocation, outages, supersession and redirects.

The release-job fixture records an HTTP side effect. It is not evidence for PostgreSQL
migrations or data recovery. CronJob readiness proves configured schedule state; it does
not assert that a scheduled execution completed. Persistent data capabilities remain M6.

## Limits and inspection

Kubernetes evidence exercises both the registrar/compiler and the API/worker/outbound
agent. This does not close M2's human auth, enrollment, UI and MCP gates. OIDC tests use
real RSA verification against a loopback test issuer and separate Ed25519 provenance.
They do not establish live GitHub App, Actions or feedback behavior. GitHub-hosted CI is
configured to repeat the disposable suite; no hosted run is claimed by local execution.

The registrar owns its generated specifications with resource-version checks, preserves
controller status/finalizers, rejects foreign/newer ownership, and retries conflicts.
Application workloads remain Flux-owned. Readiness requires actual selected workloads
and images in addition to controller generations and applied artifact revisions; an empty
successful Flux build cannot satisfy the application contract.
Failures also require the selected attempted artifact and condition generation. An old
failure cannot stand in for a newer selection. Older same-generation observations are
rejected rather than overwriting newer evidence.

Inspect only the dedicated context, for example:

```sh
mise exec -- kubectl --kubeconfig .local/kubeconfig --context k3d-keldock-m1 get kustomizations -A
```

Successful tests clean their disposable application namespaces. Cluster, registry and
database volumes remain for inspection/reuse. A failed test can leave its named namespace;
inventory and prune its Flux objects before deleting identities or the namespace. Local
HTTP and unauthenticated registry access are development settings, not a production profile.

An idle local snapshot on 2026-10-27 used approximately 1.12 GiB for the K3s node
(including Flux/system pods), 34 MiB for PostgreSQL and 30 MiB for the registry. This is
one development-machine observation, not a supported minimum, availability guarantee,
or measurement of the eventual API/agent/UI footprint.
