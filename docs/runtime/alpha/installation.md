# Install and onboard

## Inputs and target review

Bring one existing Kubernetes 1.35 cluster, Traefik, dynamic persistent storage,
an application domain, and a reachable canonical HTTPS origin. GitHub webhooks and
CI runners must reach that origin. For chart-managed ingress, supply its TLS Secret
in the installation namespace. Use the tested versions in [support](support.md); do not silently adopt
another controller version or transfer its ownership.

For TLS terminated by an operator-managed reverse proxy, set `ingress.enabled` to
`false` in the prepared values overlay. Forward the canonical origin to the console
Service on port 3800 over a reviewed private or loopback path; the API remains private.
Preserve authorization, Origin and standard MCP headers through that proxy. Run
`preflight --check-endpoint` and verify reachability from the registrar's network.
The endpoint check verifies public HTTPS trust; it does not certify the proxy's
internal transport or provide a tunnel uptime guarantee.

Use your own GitHub App and repository when connecting delivery. Keldock does not
need the maintainer's account, tunnel, cloud account, credentials or private services.
Core installation works before any repository or optional database is configured.

Review the exact context, API server, namespace, storage class and proposed RBAC.
The public ServiceAccount has no Kubernetes token or RBAC. The registrar can create
and delegate application namespaces and is a trusted operator component. The separate
database provisioner can create retained CNPG resources and read approved Secrets.
Neither is a hostile-tenant security boundary.

## Prepare reviewed inputs

Download the complete [0.6.0-alpha.1 kit](https://github.com/lucas-ht/keldock/releases/download/v0.6.0-alpha.1/keldock-0.6.0-alpha.1-kit.tar.gz)
and its [checksum sidecar](https://github.com/lucas-ht/keldock/releases/download/v0.6.0-alpha.1/keldock-0.6.0-alpha.1-kit.tar.gz.sha256).
Verify the archive before extraction, then verify `SHA256SUMS` from the extracted
kit directory. Use `sha256sum -c` on Linux or `shasum -a 256 -c` on macOS.
Put the matching OS/architecture CLI on PATH as `keldock`; the tested runtime is
Linux/arm64. The kit's `release.json` contains the public immutable product images.

For a source candidate, build
the two Dockerfiles in `packaging/images/` with `--build-arg VERSION=0.6.0-alpha.1`
and `--build-arg REVISION=YOUR_SOURCE_COMMIT`, then push them to your own accessible
registry. Record **immutable digests** in a release JSON with `version`, `revision`,
`runtimeImage` and `consoleImage`; mutable tags are rejected by the installer/chart.
Do not use fixture registry addresses from the validation record on another machine.

```sh
umask 077
keldock installation init \
  --directory ./private-installation \
  --release-file ./release.json \
  --name keldock --namespace keldock --organization my-team \
  --public-url https://control.example.com --domain apps.example.com \
  --storage-class my-tested-class --tls-secret control-tls
```

The new private directory contains installation configuration, independent random
database/admin passwords, a webhook secret, a Kubernetes Secret input, and the Helm
values overlay. Keep it out of Git and encrypted in operator backup storage.
There is no shared product administrator password.

The default development profile includes an independent PostgreSQL 18 database and
a private in-cluster Distribution registry for generated bundles. It uses private
cluster transport and NetworkPolicies; the public origin still requires HTTPS.
The chart is single replica with retained PVCs, not an HA database or registry.

For supplied PostgreSQL, add `--database-url-file ./database-uri`; outside explicit
development the URI requires `sslmode=verify-full`. For a supplied bundle registry,
add `--registry-push`, `--registry-pull`, optional `--registry-user`,
`--registry-password-file`, and `--registry-readers-file`. The two aliases must name
the same repository. The publisher and reader identities remain separate. Private
workload GHCR access uses the existing [registry reader contract](../github-delivery.md).
Operator-approved network egress must also reach supplied dependencies.
Set `networking.controlEgress` in the public values overlay to explicit Kubernetes
NetworkPolicy rules for the supplied database/registry addresses and ports. Use
`networking.agentEgress` only when agents need additional approved destinations.
Each rule requires both `to` and `ports`; review it alongside external firewall
rules. For example, PostgreSQL on a private address requires its exact `/32` and
TCP 5432. A connection URI alone does not change network authority.

Install the kit's pinned Flux manifest separately in `keldock-system`; it enables
the required impersonation/isolation flags. Cert-manager and the PostgreSQL/Barman
manifests are separate explicit choices. Inventory existing controllers first:
these files do not imply permission to replace a shared installation.

```sh
kubectl --kubeconfig ./operator.kubeconfig --context my-cluster \
  apply -f ./private-installation/installation-secret.json
keldock installation preflight --directory ./private-installation \
  --kubeconfig ./operator.kubeconfig --context my-cluster
helm upgrade --install keldock ./keldock-0.6.0-alpha.1.tgz \
  --kubeconfig ./operator.kubeconfig --kube-context my-cluster \
  --namespace keldock --values ./private-installation/values.json \
  --wait --timeout 5m
keldock installation preflight --directory ./private-installation \
  --kubeconfig ./operator.kubeconfig --context my-cluster --check-endpoint
```

Create the namespace and TLS Secret first. Preflight only reads the cluster and
checks its exact supplied dependency profile, storage and certificate. The endpoint
check verifies HTTPS trust and sign-in availability. For a private CA, provide
`--ca-file` and a chart `extraTrustSecret` containing `ca.crt` for Go processes.
It does not install global trust, prove a storage performance level or provision DNS.

## Local administrator and enrollment

Run `keldock bootstrap --config /runtime/private/control.json --email YOUR_EMAIL
--name YOUR_NAME --output /state/private/invitation.json` in the API container.
Copy the owner-only invitation securely and open its activation URL. Choose your own
password. The token expires and is consumed once. If an administrator is locked out,
the database operator can run `keldock recover --config ... --email ...
--password-file ...`; it revokes that account's prior sessions and MCP grants.

Open **Clusters**, issue an enrollment grant, and securely place its token in the
registrar container. Run `keldock enroll --config /runtime/private/agent.json
--cluster keldock --token-file /tmp/enrollment.token`. The waiting process starts
after the private credential is written. Confirm **Connected**, not merely Pod Ready.
Remove the one-time token file. An identity replacement requires archiving the old
private credential before saving its replacement; never overwrite it silently.
With `--wait-for-enrollment`, a rejected identity leaves the process waiting for
an operator-written replacement. It performs no reconciliation with the rejected
credential and loads the new identity after the file changes. Pod Ready alone
still does not mean enrolled or connected.

## GitHub, project and first release

Open **GitHub delivery** and create/connect your own compatible App. Select only
the intended repositories and verify event delivery. Follow the existing
[GitHub delivery guide](../github-delivery.md) for permissions, manifest flow and
private GHCR readers. The App's key cannot authenticate persistent GHCR pulls.

Create a private repository policy JSON containing `id`, `name`, stable numeric
`repositoryId` and `ownerId`, `workflowPath`, `pushRef`, and the public
`provenanceKeys` map. These are installation authority, not application source.
Run `keldock installation project --directory ./private-installation --file
./repository-policy.json`. Review/apply the regenerated Secret and restart the API
to load the policy. This is intentionally an operator configuration step in alpha.
After editing an existing private configuration, run `keldock installation
render-secret --directory ./private-installation` before reviewing/applying it.

Your CI builds, tests, publishes and attests immutable images. The thin Keldock Action
submits the complete release; see [the application contract](../contracts.md).
Use the published Action pinned to
`lucas-ht/keldock@04c13e53ad1f32e249027f207e104c15258aaa16`, or vendor the verified kit's
source: copy its `action.yml` and complete `action/` directory under your
application's `.github/actions/keldock/`, then use `./.github/actions/keldock` after
checkout. Keep that vendored source tied to the reviewed kit revision. Use the kit's
matching CLI binary or build the CLI from its source in trusted CI. CI retains its
own repository and registry identities.
Inspect its receipt, timeline and current-generation component health. Exercise a PR
open/update/close before enabling automatic previews for a real project.

Enable MCP deliberately in **MCP access**, register/connect the client, and consent
to specific projects. Install the bundled `keldock-preview` and `keldock-diagnose`
skills from the kit; see [agent workflows](../agent-workflows.md). Existing read-only
consent never gains mutation authority when a skill is installed.
