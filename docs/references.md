# Primary design references

Reviewed 2026-10-27. These sources inform the design; a documentation version or example
is not a selected runtime dependency. Pin and test each implemented compatibility profile.

## Delivery and reconciliation

- [Flux OCI artifacts](https://fluxcd.io/flux/cheatsheets/oci-artifacts/) and
  [OCIRepository](https://fluxcd.io/flux/components/source/ocirepositories/): immutable
  deployment artifacts and source reconciliation.
- [Flux Kustomization](https://fluxcd.io/flux/components/kustomize/kustomizations/):
  health, dependencies, pruning and service-account impersonation. Selected-release
  behavior must be proven with the chosen versions.
- [Flux multi-tenancy configuration](https://fluxcd.io/flux/installation/configuration/multitenancy/):
  scoped identities and cross-namespace restrictions. This does not establish hostile
  workload isolation by itself.
- [Crossplane managed resources](https://docs.crossplane.io/latest/managed-resources/managed-resources/)
  and [compositions](https://docs.crossplane.io/latest/composition/compositions/): optional
  declarative cloud-resource ownership. Provider-specific lifecycle support needs tests.

## GitHub integration

- [GitHub App manifests](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest):
  installation-owned App registration.
- [Actions OIDC reference](https://docs.github.com/en/actions/reference/security/oidc):
  short-lived CI identity and claims.
- [Artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations):
  artifact provenance; check supported repository visibility and plan requirements
  before choosing a mechanism. OIDC authentication alone is not artifact verification.
- [Webhook best practices](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)
  and [troubleshooting](https://docs.github.com/en/webhooks/testing-and-troubleshooting-webhooks/troubleshooting-webhooks):
  authentication, delivery handling and recovery.

## Agent interfaces

- [MCP tools, 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28/server/tools):
  schemas, discovery and results; annotations do not replace server authorization.
- [MCP authorization, 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization):
  supported remote authorization requirements, to be validated with the chosen SDK/client.
- [Agent Skills specification](https://agentskills.io/specification): portable skill
  metadata, instructions and supporting resources.

## Capabilities and positioning

- [CloudNativePG 1.30 installation and upgrade](https://cloudnative-pg.io/docs/1.30/installation_upgrade/):
  operator/instance-manager compatibility and upgrade lifecycle; not a Keldock pin.
- [Vault Kubernetes operations](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/helm/run):
  operational requirements beyond a Helm installation. Evaluate upstream licensing and
  a FOSS-compatible service profile separately.
- [Humanitec Platform Orchestrator](https://humanitec.com/products/platform-orchestrator):
  current positioning includes self-hosting and agent interfaces. Keldock's intended
  differentiation is the combination of FOSS, customer control and small-team usability.

## Repository tooling

- [actions/checkout action manifest](https://github.com/actions/checkout/blob/d23441a48e516b6c34aea4fa41551a30e30af803/action.yml):
  selected v6 commit, with persisted credentials disabled.
- [jdx/mise-action manifest](https://github.com/jdx/mise-action/blob/5228313ee0372e111a38da051671ca30fc5a96db/action.yml):
  selected v3 commit, with an explicit mise version.
- [mise 2026.10.19 release](https://github.com/jdx/mise/releases/tag/v2026.10.19):
  selected CI runtime. Tool and package pins live in .mise.toml, package.json and the lock.
