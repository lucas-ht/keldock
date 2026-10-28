# Cloud resource lifecycle

## Storage and adoption

Create allocates a deterministic account-regional S3 name. The first profile manages
private access, owner-enforced ownership, SSE-S3 encryption and enabled versioning.
It does not own arbitrary tags, lifecycle rules, replication, bucket policies or object
data. Management policies omit Delete and forceDestroy remains false.

Connect requires an exact operator-approved existing bucket and verifies its account,
region and supported security properties. Its projection is Observe-only. Connecting
and binding do not transfer the previous owner's management authority.

Adopt is a separate administrator action. First remove/freeze the previous reconciler's
ownership with retention. The operator records that release, its time and the observed
inventory hash in private policy. The API offers a review hash bound to resource and
connection revisions, account, region, bucket, inventory and release. The agent checks
that inventory again before its first managing projection. A changed inventory or stale
review is rejected; the operator must investigate and review anew. An operator release
record is an explicit assertion, not automatic discovery of every external reconciler.

Stop and retain cancels further management without deleting external data. Disconnect
removes owned projections, then releases Keldock's writer reservation only after the
agent confirms those projections are gone. Historical inventory remains visible with
external ownership. Reconnection/adoption uses a new request/review; refresh cannot
revive a disconnected writer. Remove active component/backup grants before disconnecting
storage. Preview closure does not delete retained stores or backup archives.

## Component grants and domains

A binding targets an exact existing environment generation and component set. The
provider verifies role trust, boundary and policy; the registrar then selects a new
immutable deployment with the projected identity and approved network ranges. The
original admitted image digests and CI provenance remain unchanged. Provider readiness
does not prove application data access: verify that separately using the real workload.

Disconnecting one of several stores removes that grant while preserving its sibling
grants. Preview removal tombstones its grants/claims transactionally. Late CI, retries
and agent reconnects cannot resurrect a removed environment. IAM changes are eventual;
completion requires observing the applied policy and actual AWS effects.

Domain requests select a routed HTTP component and hostname strictly below the approved
parent domain. The apex, wildcard names, foreign suffixes and duplicate active claims
are rejected. A unique TXT marker precedes the A record. Foreign records are never
silently overwritten. Ownership, record convergence and certificate readiness are
separate facts. Disconnect removes the owned address before releasing its marker. A
failed claim with no projections can be disconnected while preserving foreign records;
drift of an existing owned projection requires operator review.

## Backups and isolated restore

A backup binding targets an exact operator-supplied CNPG namespace/name/UID, pinned
PostgreSQL image, storage profile and distinct restore namespace. Existing backup/identity
fields must be explicitly released before Keldock can own those fields. Database
allocation and a general controller installer remain M6/M8 work.

The binding creates an archive generation, a base backup and a five-field cron schedule.
The source's actual identity/plugin rollout, exact ObjectStore RBAC and current archive
access gate backup creation. Status records base
backup, CNPG continuous-archiving health and actual WAL keys in the selected S3 archive.
Retry uses a new base-backup attempt. Pause/resume changes the schedule independently.
A new archive generation restricts the writer to its new prefix; older archives remain
retained, and the separate read-only restore identity receives only its selected generation.

Restore creates one isolated sandbox per binding, using a selected retained generation.
The API accepts `archiveGeneration`; the first console action selects the current one.
It cannot overwrite an existing production database or the source archive. Ready means
the restored cluster started; verify known application data before calling recovery
successful. The validation fixture proves both base data and a separately committed,
subsequently archived WAL marker. Disconnect stops schedules and source archiving and
revokes consumer grants while retaining archive data.

## Suspension and cleanup

Connection suspension rejects new intent, stops resource management and removes consumer
grants. It does not revoke the AWS provisioning roles themselves. Cloud-agent revocation
only revokes its API access; controllers and workloads retain their last approved state.
For external authority revocation, the operator must change AWS trust/permissions and
verify the effects. Trust changes block new sessions; existing sessions can require an
explicit deny. Account/zone trust roots remain operator owned.

There is no product destroy-storage action in this profile. Inventory exact targets,
account, region, owner, versions and remaining consumers before a separately authorized
external cleanup. A connection revision suspends existing projections. If a removed profile cannot be
reauthorized, the operator must finish cleanup from retained inventory before replacing
its claims. Storage versions, retained CFN resources, OIDC documents, delegation
and IAM policies need explicit handling. Never infer that uninstalling a chart or
removing a preview has erased cloud data.
