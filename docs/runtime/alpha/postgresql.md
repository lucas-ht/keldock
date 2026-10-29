# Optional PostgreSQL allocation

The `development` profile uses one retained CNPG cluster per environment/resource
alias, PostgreSQL 17.7, an empty `app` database and a non-superuser `app` role.
It is independent of PostgreSQL 18 used for control-plane state. Core delivery
requires no application database or database provisioner.

## Delegate allocation explicitly

Inventory supplied CNPG 1.30.1, cert-manager and Barman 0.15.1 against the kit pins,
or review and explicitly install the matching dependency manifests. Do not give
the public API Kubernetes credentials. Add `--postgresql` to `installation project`
to prepare an approved development profile, four retained allocation slots, the
separate database-agent configuration and its chart toggle. Review the generated
profile/storage size before applying the private Secret and upgrading the chart.

Enroll the database process independently through a project's **Resources** page:
`keldock enroll --database --config /runtime/private/database-agent.json --cluster
keldock --token-file /tmp/enrollment.token`. Registrar tokens cannot provision
databases, and database tokens cannot reconcile application workloads.

Applications declare `resources.data: {kind: postgresql, profile: development}`
and list `resources: [data]` on each authorized component. The
[acceptance application](../../../examples/postgresql/keldock.yaml) includes a web
frontend, API, worker, schedule and migration. Only bound components receive
PG connection variables, a copied username/password/CA Secret and TCP 5432 access.
TLS verifies the database hostname and CA. Database credentials never enter the
release bundle, CI receipt or MCP output.

A new preview starts empty. Closing it removes application workloads and copied
bindings, while retaining the database namespace, PVC and any archives. Retained
allocations still consume quota. Disconnect removes consumer access; reconnect is
an explicit human administrator action using the current release's declared consumers.
A release rollback does not reverse SQL migrations or restore database contents.

The development profile has one instance, bounded maintenance shutdown (15-second
smart timeout, 90-second stop delay) and no HA promise. Enabling the backup sidecar
can restart that instance and interrupt connections. Clients must reconnect; a
backup is not protected until base and WAL evidence has actually been observed.

## Supply a compatible archive

An operator approves a destination for **one exact allocation ID and CNPG source UID**
in the private control and database-agent policies. Its `backups` map records:
`allocationId`, `sourceUid`, `description`, HTTPS `endpoint`, `region`, `bucket`,
and separate `writer`/`reader` Secret references (`namespace`, `name`, immutable `uid`).
Each identity Secret has `ACCESS_KEY_ID` and `ACCESS_SECRET_KEY`; an optional `ca`
reference has only `ca.crt`. Keep actual values outside release/application input.

Scope both identities to
`keldock/ORGANIZATION/PROJECT/ALLOCATION/generation-1/*` in the supplied bucket.
Allow the writer to list/read/write/delete that prefix; allow the restore identity
to list/read only. Listing another prefix and writing with the reader must be denied.
The integration tests these permissions with two exact synthetic probe keys and
cleans those keys up. It never issues archive-object deletion or bucket management.
Automatic storage provisioning belongs to the separate optional cloud extension.

After applying/reloading both policies, a human administrator selects **Enable
backups** in Resources. This configures the native plugin, requests a base backup,
enables daily scheduled bases, and observes the source's archive health and actual
WAL objects. Report base completion, last archived object time and observation
freshness separately. No evidence means no backup claim.

Select **Isolated restore** only after fresh ready backup evidence. A separate
`kr-*` namespace and CNPG cluster use the read-only identity and cannot receive
application connections. Cluster Ready is not data verification: query a known
committed marker, including one written after the base backup, before planning a
cutover. No automatic cutover or source replacement exists in alpha.

Pause stops scheduling and disconnects WAL archiving; archive objects remain retained.
Retention/deletion and off-machine protection belong to the supplied storage operator.
Missing or changed source UIDs block reconciliation instead of creating an empty
replacement under the same database identity.
