# Keldock agent guide

## Mission

Build a portable FOSS application and infrastructure control plane for individuals,
small teams, and larger organizations. Customer-controlled infrastructure comes first.
The product combines multi-service delivery, modular capabilities, and shared API/MCP
interfaces; an optional hosted control plane and hosting follow later evidence gates.

This is a fresh repository. Only implemented code and recorded validation in this
checkout count as current evidence. Do not claim historical prototypes still exist.

## Working rules

- Be direct about uncertainty and challenge unsupported assumptions.
- Read the nearest subtree guide before editing; preserve unrelated changes.
- Keep files focused, generally below 300 lines, and organize growing directories.
- Research complex work against current primary sources; cite them in design notes.
- Treat docs/architecture.md, its decisions, and stable Task interfaces as contracts.
- Pin tools, actions, providers, charts, images, and libraries; update related locks.
- Add runtime dependencies and directories only with a working implementation. Do not
  create placeholder services, charts, tools, or skills that imply a usable product.
- Commit coherent layers with conventional prefixes such as chore(repo):,
  docs(architecture):, feat(control-plane):, feat(agent):, or feat(capabilities):.

## Product boundaries

- Retain AGPLv3 for the reusable product. The self-hosted core, API, authorization,
  recovery, MCP adapter, and first-party skills must work without a paid account.
- Keep private installation configuration, accounts, clusters, credentials, Terraform
  state, and real application instances outside this product repository.
- Initial scope is one organization and existing customer-controlled cluster, GitHub
  delivery, multi-service previews, optional PostgreSQL, and bounded MCP/skills access.
- Builds, tests, and image publication belong to user CI. The first Keldock Action is
  thin, consumes verified prebuilt image digests, and submits deployment requests.
- Applications need source/build inputs, keldock.yaml, and a CI submission step; no
  Flux manifests, deployment Git repository, Helm/Terraform, or cluster credentials.
- Flux reconciles application workloads through application-scoped identities. Keldock
  owns selection, registration, and its generated Flux configuration.
- Optional Crossplane reconciles only approved cloud resources through namespaced
  managed resources in the selected provider profile. No two reconcilers may
  continuously own the same resource.
- Separate capability installation, resource allocation, and application bindings.
  Controller management requires explicit operator delegation and one active owner.
- Keep public endpoints, workload reconciliation, and privileged provisioning in
  separate trust domains. Application source cannot grant administrative authority.
- UI, CI, MCP, and skills use the same authorized product APIs. Skills and tool
  annotations cannot grant privileges or replace server-enforced policy.
- Version desired state; match observed health to the requested revision. Persist work
  durably so accepted operations survive client, worker, and agent interruption.
- Separate resource retention from release/preview deletion. Connecting a service does
  not transfer management ownership.
- Default installation must not require AWS, Crossplane, Vault, Temporal, an enterprise
  identity provider, or an LLM provider. Validate a PostgreSQL-backed worker first.
- Never commit secrets, keys, kubeconfigs, state, plans, or runtime logs.

## Operations and completion

Run narrow checks while iterating and task doctor for repository changes. Add a real
task local:test with the first runtime fixture; never add a successful placeholder.
Runtime-affecting changes then require that local acceptance path.

Report static, local-runtime, live-integration, cloud, and recovery evidence separately.
Resolve exact targets and obtain explicit operator approval before production mutations,
cloud applies, destructive operations, state migration, or publication. No bootstrap
work authorizes modifying existing platform/application deployments.
