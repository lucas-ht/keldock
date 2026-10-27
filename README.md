# Keldock

Keldock is an experimental free and open-source control plane for deploying and operating
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

The experimental runtime includes a Go API, durable PostgreSQL worker, OCI publication,
outbound agent and scoped Flux reconciliation. [Accounts and enrolled identities](docs/runtime/authentication.md)
now protect the [Next.js console](docs/runtime/console.md) and five read-only MCP tools.
The console leads with deployment health, with separate people, cluster and MCP access
management. [Disposable tests](docs/validation/m2-access-and-console.md) exercise real
PostgreSQL, Kubernetes, HTTP and an OAuth-enabled MCP SDK client.

Private registry pull profiles, live GitHub integration, the thin Action, MCP writes,
skills and general installation retain their milestone gates. This is not a packaged
self-hosted alpha or a production-validation claim.

The first release targets one organization and cluster, GitHub delivery, multi-service
previews, optional PostgreSQL with tested recovery, and a small MCP/skills surface.
General module installation, automated AWS provisioning, hosted tenancy, and compute
fleets are later milestones.

## Design and roadmap

- [Product direction](docs/product/direction.md): audience, FOSS commitments and business direction.
- [Architecture](docs/architecture.md): components, ownership and deployment boundaries.
- [Implementation plan](docs/implementation-plan.md): current implementation and remaining milestone gates.
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

`task doctor` checks repository contracts, formatting, Markdown links, Go checks,
workflow syntax and the console's TypeScript/production build. It does not validate a
Kubernetes deployment or cloud access.
The disposable developer environment is available through `task local:test`; see
[process commands and private configuration](docs/runtime/processes.md).
There is no general runtime installer yet.

For the local console, run `task local:console:api` and `task local:console` in separate
terminals, then open `http://127.0.0.1:3800`. The private fixture login and exact local
targets are documented in the [console guide](docs/runtime/console.md).

See [contributing](.github/CONTRIBUTING.md), [security](.github/SECURITY.md), and the
[agent guide](AGENTS.md). The repository retains its [AGPLv3 license](LICENSE).
