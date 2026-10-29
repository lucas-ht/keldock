# Self-hosted alpha candidate

M6 packages one organization and one customer-controlled cluster. The Go API and
worker share a pod with the console; the registrar and optional PostgreSQL
provisioner have separate enrolled identities. No AWS, Crossplane, enterprise SSO,
Vault, Temporal, paid Keldock account or model provider is required.

This is an **unpublished candidate**. The fresh-target operator walkthrough and
release publication remain explicit gates in the [implementation plan](../../implementation-plan.md).
Use the matching source, chart, image digests and checksums from one reviewed kit.

- [Install and onboard](installation.md), including supplied dependencies.
- [PostgreSQL allocation and supplied backups](postgresql.md).
- [Control-plane recovery and uninstall](recovery.md).
- [Supported profile and limitations](support.md).
- [Fresh-target operator walkthrough](walkthrough.md), with assistance recorded.
- [MCP and portable skills](../agent-workflows.md).

The first view stays centered on environments and deployment health. Administrators
can open **Setup** for current account, HTTPS configuration, agent connection,
GitHub delegation, project policy and first-environment evidence. A checklist is not
a backup certificate or an authorization grant.
