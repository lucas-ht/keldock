# Modular capabilities

Status: PostgreSQL allocation/bindings and the optional AWS profile have concrete runtime contracts.
The general module catalogue, controller lifecycle API and extension SDK remain design gates.

## Four separate responsibilities

A capability is a supported service an app consumes. A module is the versioned
integration that exposes and, when authorized, installs or manages that service.

| Concept                | Responsibility                                                                                                           |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| CapabilityDefinition   | Typed configuration/binding schemas, implementation/version, dependencies, permissions, licenses and supported lifecycle |
| CapabilityInstallation | Exact target, version, desired revision, management mode, owner, observed health and operation history                   |
| Resource               | Project-authorized database, bucket, secret access scope or workflow namespace with independent retention                |
| ResourceBinding        | Exact consumer access, endpoints and secret/workload-identity references                                                 |

Installing an operator, creating a database, and giving an application access are
different operations. A shared installation can serve several projects without sharing
their data or permissions. Define executable contracts in M1; these names are not shipped
CRDs or an extension SDK.

## Authority

The installation administrator authorizes definitions, versions, profiles, targets,
capacity and privileges. Resource requests within those bounds can reconcile
automatically. Shared-controller installation, new authority, adoption, and persistent
data deletion are separate operator operations.

V1's explicit bootstrap owns only dependencies it installs. Existing compatible operators
retain their current owner. Later, a capability manager may receive delegated ownership
for selected installations. Never compete with operator GitOps, Helm, Terraform or
another control plane.

Installation intent goes through the durable API and outbound agent to a separate
installation identity. Application Flux identities cannot install controllers or edit
their grants. No raw Helm values, executable plugin hooks, kubectl or remote shell is
accepted as an application or MCP capability request.

Use existing reconcilers with one writer per object. Flux retains application workloads;
optional Crossplane owns its approved cloud resources. Select/pin the installation
reconciler only after coexistence, permissions and recovery tests.

## Modes and lifecycle

- **Supplied:** validate/discover an operator-managed installation; bind approved
  consumers without changing its version or lifecycle.
- **Managed:** install/operate under explicit delegated authority and immutable approved
  configuration revisions.
- **External:** connect a supported service outside the cluster while its existing owner
  retains lifecycle unless separately delegated.

Connecting is not adopting. Transfer requires an exact inventory, frozen old writer,
approved managed fields and retention, preserved identity, and verification before
resuming. Disconnecting a binding, disabling allocation, uninstalling a controller, and
destroying data are distinct actions.

Each managed integration declares and tests:

1. Pinned artifacts, licenses/provenance, Kubernetes/agent/controller/service/schema
   compatibility and unsupported combinations.
2. Acyclic dependencies, storage/network/identity/capacity prerequisites, bootstrap order,
   and operator actions that automation cannot complete.
3. Configuration admission, reviewable desired revision, operation ID, idempotency,
   cancellation and recovery from partially completed external effects.
4. Usable service/binding readiness tied to the selected revision, including missing
   prerequisites, waiting for approval, degraded/disconnected and retained outcomes.
5. Upgrade impact, backup/restore, irreversible migration limits and controlled rollout.
   Installing an older chart is not a data rollback guarantee.
6. Disable/disconnect/uninstall/retention semantics, remaining resources/finalizers and
   blocked removals when consumers or dependencies remain.

A chart install is only one part of operating a service. [Vault][vault] documents the
remaining operational responsibility, and [CNPG][cnpg] distinguishes controller and
instance-manager upgrades. Keldock needs its own version-specific lifecycle evidence.

## First capability: PostgreSQL

Reuse a compatible supplied CNPG operator or install a pinned one through an explicit
bootstrap profile. Implement database creation, consumer isolation, bindings, previews,
retention, compatible upgrades, backup and demonstrated isolated restore.

Use one tested backup integration and a supplied compatible destination with scoped
identity. Provisioning that infrastructure automatically is optional M5. Backup status
requires recent base-backup/WAL evidence; recovery requires an actual restore. Maintain
separate writer/restore identities and archive generations. Disposable fixtures do not
prove durable recovery. External PostgreSQL binding needs separate validation.

Control-plane PostgreSQL is a required service with an independent bootstrap/recovery
path. It must not rely on its own running API or the optional application database module.

V1 exposes inventory/status and this integration. It has no general remote controller
installation API, plugin marketplace, or executable third-party extension system.

## Future modules and cloud resources

Remote installation/upgrade comes first for PostgreSQL, then one additional integration
before freezing a general extension API. UI and MCP use the same typed APIs and policy.

Secret management, including possible Vault integration, needs initialization,
seal/recovery material, authentication, rotation, backup and restore. A fully FOSS
installation must remain available; evaluate upstream licenses separately.
Temporal needs persistence, schema changes, authentication, namespaces and safe upgrades.
Customer Temporal remains independent of Keldock's internal worker engine.

For optional AWS, a CloudConnection specifies account, region, allowed profiles and a
designated provisioning identity. Use exact short-lived workload trust; do not assume
EKS or a public OIDC issuer. Apps cannot submit raw IAM policies or choose arbitrary
roles/provider configurations. Validate each provider's management/retention behavior.
Keep resources outside preview deletion trees, and distinguish create, connect, adopt,
disconnect and approved deletion. Apps access storage/services directly; Keldock does
not proxy their data traffic.

[vault]: https://developer.hashicorp.com/vault/docs/deploy/kubernetes/helm/run
[cnpg]: https://cloudnative-pg.io/docs/1.30/installation_upgrade/
