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

The experimental core M1 is implemented: [runtime components](docs/runtime/contracts.md)
include an HTTP API, durable PostgreSQL worker, OCI compilation/publication, outbound
agent and scoped Flux reconciliation. [Disposable runtime tests](docs/runtime/local-development.md)
exercise GitHub identity/event fixtures, multi-service previews, API-only image updates,
failure/recovery and teardown through the API and compiled processes. Live GitHub,
product auth/enrollment/UI, the Action, MCP and general installation remain later gates.

The first release targets one organization and cluster, GitHub delivery, multi-service
previews, optional PostgreSQL with tested recovery, and a small MCP/skills surface.
General module installation, automated AWS provisioning, hosted tenancy, and compute
fleets are later milestones.

## Design and roadmap

- [Product direction](docs/product/direction.md): audience, FOSS commitments and business direction.
- [Architecture](docs/architecture.md): components, ownership and deployment boundaries.
- [Implementation plan](docs/implementation-plan.md): M0 and core M1 implemented; product milestones follow.
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
The disposable developer environment is available through `task local:test`; see
[process commands and private configuration](docs/runtime/processes.md).
There is no general runtime installer yet.

See [contributing](.github/CONTRIBUTING.md), [security](.github/SECURITY.md), and the
[agent guide](AGENTS.md). The repository retains its [AGPLv3 license](LICENSE).
