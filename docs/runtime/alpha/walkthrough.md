# Fresh-target operator acceptance

The automated local fixtures are developer evidence. Before a published alpha,
complete this journey on a fresh target using only a matching kit and these guides,
an operator-owned cluster, domain, GitHub App/repository, identities and supplied
backup service. Do not depend on maintainer fixture credentials or addresses.

The operator may execute it manually or explicitly delegate execution to an agent,
retaining sign-in, credential creation and required access approvals. Record an
agent-run journey as assisted acceptance. It does not establish independent human
usability evidence, and an automated fixture rerun does not replace this journey.

Record the kit version/source revision/checksum, target Kubernetes/storage/ingress,
operator, executor and date, commands, results, confusing instructions and assistance.
Keep tokens, connection strings, private configuration and customer data out of the
report. Mark each step failed, assisted or passed honestly.

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
