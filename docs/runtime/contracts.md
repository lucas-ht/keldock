# First runtime contracts

Status: experimental core M1 implemented. Go contracts, validation, OIDC/provenance
verification, durable dispatch, workload compilation, HTTP API and outbound agent exist.
This is not yet a general installation. See [local evidence](local-development.md).

## Implemented profile

The experimental version is `keldock.io/v1alpha1`. The authoritative types and validators
are in internal/contracts/v1alpha1. JSON requests reject unknown fields, duplicate keys
and trailing values; YAML application definitions also reject duplicate/unknown fields
and multiple documents.

An application has 1–16 named components. Supported kinds are http, worker, job and
cronjob, with the small size profile. Long-running workloads require an HTTP or exec
probe. One component can receive an ingress route. Prerequisite dependencies must form
an acyclic graph; communication edges can be cyclic and generate explicit network rules.
Build inputs remain outside this definition.

Releases require every component's immutable sha256 image reference, exact source and
workflow/run identity, and signed provenance. These structural checks do not establish
identity or trust by themselves; admission must separately verify CI authentication,
repository/target policy and the approved signing key.

## Provenance decision

The first profile uses [DSSE](https://github.com/secure-systems-lab/dsse/blob/master/protocol.md)
with Ed25519 and an operator-approved public key scoped to a repository/workflow. This
works without a public transparency-log entry or a paid repository plan. The signing
step belongs to trusted user CI; the thin deployment Action only consumes evidence.
Keys must be isolated from untrusted fork code, scoped and rotatable.

The signed [SLSA v1](https://slsa.dev/provenance/v1) profile binds the complete artifact
subjects, application definition digest, project, source, workflow and run attempt.
Verification uses the exact signed payload bytes. It rejects changed images, registry
names, application input, project, repository, run and workflow. This is a specific
supported provenance profile, not a claim of a SLSA assurance level or hermetic build.
GitHub OIDC remains a separate authenticated-submitter requirement.

## Compilation and publication

The compiler emits deterministic tar+gzip content containing only Deployments, Jobs,
CronJobs, Services, Ingresses and NetworkPolicies, plus Kustomize build files. Trusted
placement supplies the namespace, environment, deployment generation and route hostname.
Namespace creation, accounts, RBAC and Flux configuration belong to the registrar.
The build file is named kustomization.yaml; its JSON content is valid YAML.

Workload pods run as non-root with read-only root filesystems, dropped capabilities,
resource bounds, disabled service-account mounts and bounded temporary storage. Network
policy permits DNS and explicit component connections. Runtime enforcement requires the
disposable-cluster tests; generated YAML alone does not prove isolation.

Release jobs receive a stable identity per deployment, so retrying the same selection
does not deliberately create another job. Selecting another deployment runs its jobs
again. Flux uses its
[IfNotPresent apply policy](https://fluxcd.io/flux/components/kustomize/kustomizations/#ifnotpresent)
for these immutable Job objects: a late registrar pull patch cannot change a running
or completed Pod template or recreate its migration. Job Pods inherit the
operator-owned ServiceAccount's reader when admitted. Each Job Kustomization uses
an explicit health check for the selected Job, with `wait: false`, following
[Flux's health-check contract](https://fluxcd.io/flux/components/kustomize/kustomizations/#health-checks).
The registrar advances the source digest only after all component policies target
the new selection. Observed readiness also requires every selected prerequisite
to be ready, including transitive dependencies; an independent branch stays usable.
Application job code must be idempotent. CronJob readiness describes configured
schedule health, not proof of a successful scheduled execution.

OCI content and manifest digests are deterministic. Content-derived tags retain bundles;
delivery uses the digest. Publication tolerates existing blobs/config/manifests. Registry
push credentials and application image publication remain separate responsibilities.

See the [implementation plan](../implementation-plan.md) for remaining M1 runtime gates.
