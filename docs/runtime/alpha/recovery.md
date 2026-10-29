# Control-plane recovery and exit

Back up control-plane state independently of application databases. A PostgreSQL
allocation backup does not contain users, admitted releases, selection history,
agent enrollment, private installation files or generated OCI bundle graphs.

## Capture and verify

Quiesce administrative/configuration changes and credential rotation for the snapshot.
The supported profile has a dedicated one-organization control-plane database.
Run the matching release CLI with PostgreSQL 18 `pg_dump`/`pg_restore` clients on its
PATH (already included in the runtime image):

```sh
umask 077
keldock recovery backup --config ./private/control.json \
  --registry-credentials-file ./private/registry-pulls.json \
  --directory ./new-snapshot
keldock recovery verify --directory ./new-snapshot
```

Omit the reader flag only when referenced workload images require no credentials.
The snapshot uses one exported repeatable-read PostgreSQL snapshot, copies the
referenced private installation material, and exports every referenced bundle and
workload image graph into an OCI layout. Missing artifacts fail the capture.
It rejects credentials changed during capture and inventories all members/checksums.
The manifest is integrity metadata, not encryption or an independent signature.

The directory contains secrets. Encrypt it and store it off-machine under operator
control. Keep the storage class/volumes, TLS/DNS, supplied identity Secrets, registry
administration and dependency-controller backups in their own recovery plan.
Agent credentials are not recovered as valid authority.

## Restore against surviving infrastructure

Stop the old API, worker and all enrolled agents first. Confirm that only one
control plane can resume for this organization/cluster. Preserve surviving workloads,
the `keldock-system` checkpoint ConfigMaps, retained database namespaces/PVCs and
their source UIDs. Do not reconcile from an older snapshot to discover what it deletes.

Create an **empty independent** PostgreSQL target, owned by the intended database
role. Copy/review private installation settings for that target without overwriting
the original source or printing its connection URI. Then:

```sh
keldock recovery restore --config ./restored/control.json \
  --directory ./snapshot
keldock recovery restore-bundles --config ./restored/control.json \
  --directory ./snapshot
keldock recovery review --config ./restored/control.json \
  --kubeconfig ./operator.kubeconfig --context my-cluster
```

Restore installs a startup fence before recovered schema/state can commit. The
restored server refuses startup. Browser sessions, pending invitations/enrollments,
MCP grants and all three agent-purpose identities are revoked; operation leases
are cleared. Restored passwords/account/project grants still require operator review.

The review compares desired generations, deployment/bundle identity, deleted-preview
tombstones and retained database generations/source UIDs against surviving cluster
checkpoints. Its deterministic output includes nonsecret account roles/status and
project grants. Review it against current external/operator records and reapply any
post-snapshot access revocations before resuming. An older/conflicting snapshot
stays fenced: restore a newer snapshot instead of deleting newer cluster state.

`restore-bundles` writes only the configured Keldock bundle repository, preserving
digests. Workload image graphs remain in the OCI layout for their registry owner
to republish at the original immutable addresses. Verify pull availability before
resuming. This is not a general registry migration or private-key rotation tool.

After exact target/account/artifact review, run `keldock recovery resume` with the
same config/kubeconfig/context and the returned `--review-hash`. A changed inventory
invalidates the hash. Restart the reviewed server and explicitly enroll new registrar,
database and optional cloud identities. Sign in again and reconnect MCP with fresh
consent. Verify the current selected generation, real workload health, a retained
data marker and backup/archive health.

This procedure handles control-plane loss with surviving cluster state. Complete
cluster/volume loss, replacement source UIDs, automatic data cutover, application
rollback of SQL and cross-cluster disaster recovery require separate operator plans.

## Upgrade and uninstall

Before upgrading, take and verify a private snapshot and review migrations, image
digests and upstream dependency release notes. Apply one coherent chart/image kit.
Migrations are checksummed and forward-only; do not roll an old binary over a newer
schema. A single replica uses a recreate rollout and has a control-plane interruption.
Running workloads retain their last approved state while the API is down.

Inventory the exact Helm release/context/namespace, workload namespaces, retained
allocations/archives, PVCs, checkpoint ConfigMaps and external identities before an
uninstall. Revoke agent/MCP authority and stop the processes, then uninstall only
the reviewed Keldock release. The chart marks its state/database/registry PVCs
`helm.sh/resource-policy: keep`. Application and database namespaces and checkpoint
ConfigMaps are not Helm children and remain retained. Shared/supplied controllers
and external archives are not removed.

Do not delete the installation namespace or retained PVCs as a cleanup shortcut.
That bypasses Helm's keep annotation. Physical deletion, storage retention and
external role revocation are separate, explicitly approved operator actions.
