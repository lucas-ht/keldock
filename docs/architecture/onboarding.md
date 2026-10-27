# Self-hosted onboarding

Status: required M6 experience; no installer or runnable product package exists yet.
M1 selects packaging and authentication through working spikes.

## Intended first installation

An individual or small team brings one existing Kubernetes cluster. Start with an
all-in-one control-plane/agent profile; supplied dependencies remain available where
tested. Prefer one documented path with measured requirements over a matrix of
untested combinations. A single-node development profile does not promise availability.

The operator supplies cluster access, supported storage/networking, DNS and reachable
HTTPS for the UI/API and GitHub webhooks, and registry publication/pull access. CI
runners must reach the submission endpoint. Report these requirements before installation.
Do not require a new AWS account, enterprise SSO, Vault, Temporal or paid Keldock account.

## Journey

1. **Preflight:** check exact cluster/context, versions, storage, ingress, certificates,
   registry access and capacity. Inventory existing controllers and owners. Show what
   bootstrap would install and what it will only use; never adopt silently.
2. **Bootstrap:** install approved pinned dependencies, independent control-plane
   PostgreSQL, API/worker, agent/controllers and the initial UI. Create a local
   administrator with a documented recovery path and no shared default password.
3. **Authorize:** create the organization/project, grant repository/environment access,
   set domains, preview limits and production promotion policy. Enroll the cluster and
   display granted capability scope, version compatibility and observation freshness.
4. **Connect GitHub:** register the installation-owned App through its manifest or
   supply an existing compatible App. Select repositories and store/rotate credentials.
   Keep App installation and human login distinct.
5. **Connect an application:** validate keldock.yaml, map the monorepo's components,
   configure routes/resource profiles, and provide a thin Action integration for
   existing CI. If requested, prepare a setup PR with additional permission explicitly
   granted for that operation; otherwise give the user a patch.
6. **Deploy and preview:** CI builds/tests/publishes, then submits verified immutable
   releases. Show a durable receipt, timeline, component health and GitHub feedback.
   Open/update/close a PR and observe preview creation, supersession and cleanup.
7. **Use agents:** configure the authenticated MCP client and install compatible skills.
   Discover, deploy an admitted release to nonproduction, inspect a failure and retry
   through the same policy as the UI.
8. **Recover:** demonstrate backup/restore, worker/agent interruption, revocation,
   retained-resource inspection and a controlled upgrade before calling the install
   production-ready.

## Optional PostgreSQL profile

Core delivery works without an application database. When requested, use a compatible
supplied CNPG operator or explicitly install the tested version. Supply a supported
backup destination and scoped identity; cloud provisioning is a separate extension.

Expose database availability, authorized bindings, migration effects, preview isolation,
retention, backup/WAL status and an actual isolated restore. Keep the control-plane
database's recovery path independent of the application capability.

## Completion and measurement

The acceptance application includes web, API, worker, schedule, migration and PostgreSQL.
Record setup steps/time, required accounts, steady-state resources, failed prerequisites,
recovery results and ongoing operator responsibilities. Test with someone who did not
build the product, using no private developer installation or hidden manual fixes.

Uninstall must inventory retained data, identities, shared/supplied controllers and
external services. Removing Keldock is not authorization to delete customer data.
Publish an exit/recovery procedure alongside the install procedure.

See [delivery](delivery.md), [capabilities](capabilities.md), and
[security and operations](security-and-operations.md).
