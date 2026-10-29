# Independent operator acceptance

The automated local fixtures are developer evidence. Before a published alpha,
a separate human operator must complete this journey using only a matching kit
and these guides, their own cluster, domain, GitHub App/repository, identities and
supplied backup service. Do not reuse maintainer fixture credentials or addresses.

Record the kit version/source revision/checksum, target Kubernetes/storage/ingress,
operator and date, commands, results, confusing instructions and required assistance.
Keep tokens, connection strings, private configuration and customer data out of the
report. Mark a step failed or assisted honestly; an automated rerun is not a human
walkthrough.

1. Verify kit checksums and source, then follow [installation](installation.md).
   Install core without CNPG or cloud controllers. Activate a local account,
   verify administrator recovery and enroll the registrar over canonical HTTPS.
2. Connect a scoped GitHub App and project policy. Submit the multi-service example
   from customer CI, inspect the receipt and wait for fresh matching-generation
   health. Open/update/close a preview without authoring Flux manifests.
3. Connect an MCP client with read consent and install both bundled skills. Inspect
   deployment/resource state; verify an unauthorized project and a mutation without
   operation consent are denied.
4. Explicitly enable the [PostgreSQL development profile](postgresql.md). Deploy the
   database example, write a staging marker and prove a new preview starts empty.
   Close the preview and inspect its retained database.
5. Supply separate scoped archive writer/reader identities. Complete a base backup,
   commit another marker, observe archived WAL and verify that marker by querying
   the isolated restored cluster. Backup status alone does not pass this step.
6. Follow [control-plane recovery](recovery.md) into an empty independent database.
   Verify snapshot contents, revoked recovered credentials, surviving workload
   availability, the stale-state fence and explicit new enrollment after review.
7. Upgrade one coherent kit, then inventory and uninstall only the Keldock release.
   Check retained PVC identities, application/database namespaces and archives.
   Exercise supplied PostgreSQL/registry installation independently of Helm-owned
   dependency lifecycle.

Return the sanitized report to the maintainer for review. Publication is a separate
explicit decision after the walkthrough and an operational security-reporting route
are confirmed. This document does not assert that either gate has passed.
