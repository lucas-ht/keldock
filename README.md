# Keldock

Keldock is a planned free and open-source control plane for deploying and operating
complete applications on customer-controlled infrastructure. It is intended for
individuals, small teams, and larger organizations, with shared APIs for people and
AI agents.

Bring an existing Kubernetes cluster and your CI. Keldock will turn a versioned
application definition and prebuilt images into observable deployments, isolated
previews, and resource bindings. Modular capabilities add supporting services; MCP
and portable agent skills expose the same authorized workflows.

The long-term direction is an optional hosted control plane using the same FOSS core,
followed by managed infrastructure and potentially hosting. Self-hosted operation must
remain complete without a paid account or enterprise identity provider.

## Status

M1 is in progress: [runtime contracts](docs/runtime/contracts.md), validation, signed
provenance verification, workload compilation and OCI publication are implemented as
Go libraries. The product server, agent, MCP endpoint, GitHub Action and installable
package are still pending.

The first release targets one organization and cluster, GitHub delivery, multi-service
previews, optional PostgreSQL with tested recovery, and a small MCP/skills surface.
General module installation, automated AWS provisioning, hosted tenancy, and compute
fleets are later milestones.

## Design and roadmap

- [Product direction](docs/product/direction.md): audience, FOSS commitments and business direction.
- [Architecture](docs/architecture.md): components, ownership and deployment boundaries.
- [Implementation plan](docs/implementation-plan.md): M0 completed; M1 contracts and spikes in progress.
- [Expansion roadmap](docs/product/roadmap.md): modules, hosted control plane and hosting.
- [Decisions](docs/decisions/README.md), [primary references](docs/references.md), and
  [validation evidence](docs/validation/README.md).

## Development

Use the versions in [.mise.toml](.mise.toml):

```sh
mise install
mise exec -- task bootstrap
mise exec -- task doctor
```

`task doctor` checks repository contracts, formatting, Markdown links, workflow syntax,
and validation-tool tests. It does not validate a Kubernetes deployment or cloud access.
There is no runtime installation command yet.

See [contributing](.github/CONTRIBUTING.md), [security](.github/SECURITY.md), and the
[agent guide](AGENTS.md). The repository retains its [AGPLv3 license](LICENSE).
