# Optional AWS development profile

M5 adds approved AWS connections, private object/backup stores, component identity,
Route53 claims and backups for an operator-supplied CloudNativePG database. Core delivery
continues to work with no cloud configuration, AWS account, Crossplane or paid service.
This is an explicit operator profile; general installation/onboarding remains M6.

See [setup and trust](setup.md), [resource lifecycle](lifecycle.md),
[the architecture decision](../../decisions/0007-optional-aws-profile.md) and
[recorded validation](../../validation/m5-aws-resources.md).

The console keeps deployment/environment health as its main overview. An enabled
project gains **Cloud resources**, with connection verification, inventory, component
access, domains and supplied-database backup controls. Viewers can inspect; operators
can request approved resources/bindings; administrators manage connections and adoption.
The installation operator controls the available accounts, regions, profiles, identities,
DNS zone, database targets and network ranges. Application source cannot expand them.

These operations use the same session/CSRF and transactional authorization boundaries
as the rest of the product. Every mutation requires an idempotency key and returns a
receipt for a desired generation; acceptance does not mean AWS effects completed.
Observations identify their resource and connection generations and observation time.
Stale observations cannot declare a newer revision ready.

## API surface

| Endpoint                                      | Purpose                                                           |
| --------------------------------------------- | ----------------------------------------------------------------- |
| GET /v1/projects/{project}/cloud              | Authorized inventory, approved choices and agent health           |
| GET /v1/projects/{project}/cloud/intents/{id} | One retained intent and its current evidence                      |
| POST /v1/projects/{project}/cloud             | Typed connection/resource/binding/domain/backup/lifecycle command |
| /v1/cloud/clusters/{cluster}/enrollment       | Independent cloud-agent enrollment                                |
| /v1/cloud/clusters/{cluster}/revoke           | Revoke cloud-agent control-plane access                           |

The POST body is `{ "kind": "resource", "id": "uploads", "input": { ... } }`.
Connection input selects approved account/region/profile/identity references. A resource
selects its connection, kind, profile and create/connect mode. A component binding selects
an existing environment generation, resource, component names and read/read-write access.
It cannot supply role ARNs, Kubernetes namespaces, object prefixes or policy documents.

Cloud mutations require human sessions. The existing M4 MCP `keldock:operate` grant and
skills retain their nonproduction deployment/retry/cancel scope; they gain no cloud,
adoption or administration privileges. A future cloud MCP profile needs separate typed
consent, authorization and lifecycle evidence.
