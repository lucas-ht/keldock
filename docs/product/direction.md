# Product direction

Updated 2026-10-27. Accepted direction; an experimental core runtime exists.
The complete product and expansion tracks retain separate acceptance gates.

## Promise

Keldock is a free and open-source control plane for deploying and operating complete
applications and their supporting infrastructure. People and AI agents use the same
authorized APIs. Customers can operate it themselves; a future hosted control plane
serves their clusters through the same protocol.

Start with individuals and small teams that have a Kubernetes cluster and want useful
application workflows without assembling an internal platform or buying an enterprise
engagement. Larger organizations can use the same product. Kubernetes remains a real
adoption cost; measure setup effort and resource requirements before claiming simplicity
or low operating cost.

## Commitments

- **Customer control:** existing clusters, application data, infrastructure accounts,
  resource ownership, and recovery remain under customer control. No required hosted
  identity, subscription check, or call-home service in the self-hosted product.
- **FOSS:** retain AGPLv3 for the reusable control plane, agents, first-party adapters,
  MCP integration, and skills. A complete supported self-hosted path remains usable
  without a commercial subscription.
- **Accessible operation:** API access, RBAC, audit, backup/recovery, and automation are
  normal product features. No enterprise identity provider or sales engagement is
  required to get started.
- **Small required stack:** a modular API/worker, PostgreSQL, an OCI registry, and the
  supported workload controllers. Additional services are optional capabilities.
- **Predictable automation:** typed requests, stable operation IDs, idempotency,
  observable progress, bounded errors, and evidence tied to the requested revision.
- **Portable operation:** document export, recovery, and ownership transfer. Workloads
  retain the last approved state during a control-plane outage.

Third-party integrations retain their own licenses. Record upstream provenance and
distribution constraints per capability. A fully FOSS installation path is required;
supporting a service does not change its upstream license.

## AI and differentiation

Initially, AI accessibility means agents can operate applications and infrastructure.
Applications using AI naturally fit that model. GPU fleets, model serving, and executing
arbitrary untrusted code are separate future problems.

MCP exposes product operations and skills teach their use. The value is a complete,
reliable workflow: deploy an approved release, diagnose a failed rollout, recover, and
remove an environment without losing retained data.

Differentiate through the combination of FOSS, customer control as the default, and
practical use by individuals and small teams. [Humanitec][humanitec] already advertises
agent interfaces and self-hosting. Do not claim those individual features are unique,
or make unsupported claims about competitors' licensing and pricing.

## Commercial progression

1. Prove the self-hosted application platform and recovery with external users.
2. Offer an optional hosted control plane on customer-owned clusters, using the same
   FOSS core. Sell managed availability, upgrades, support, and fleet convenience.
3. Consider managed infrastructure in customer cloud accounts once scope, costs,
   ownership, and recovery are proven.
4. Consider provider-operated hosting with separate capacity, isolation, abuse handling,
   metering, support, and economic evidence.

Pricing, willingness to pay, and service levels remain business hypotheses. A broad
module catalogue or compute fleet is not needed to test demand for hosted operations.

See [v1 implementation](../implementation-plan.md), [later milestones](roadmap.md), and
the [architecture](../architecture.md).

[humanitec]: https://humanitec.com/products/platform-orchestrator
