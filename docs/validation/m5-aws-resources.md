# M5 AWS validation

Validation date: 2026-10-28. M5 is accepted for this bounded optional development
profile. General external-operator alpha installation remains M6. All cloud work used an explicitly
approved disposable prefix and delegated child zone in the operator's personal AWS
account, region eu-central-1. Real account/resource configuration and raw logs remain
in private installation storage, outside Git. No platform/production cloud apply ran.

## Static and local evidence

- Policy/compiler tests cover account/region/profile boundaries, exact component
  identities, bounded public egress, no arbitrary IAM/namespace/prefix injection,
  resource retention, provider pins and observation generations.
- PostgreSQL integration tests cover independent registrar/cloud enrollment and
  revocation, durable receipts, quotas, stale revisions, released-writer adoption,
  confirmed disconnect, and denial of cloud writes to existing M4 MCP grants.
- Desktop/light/dark inventory, real status/failure states, backup/restore details and
  setup/cancellation dialogs were inspected in the built-in browser. The 390×844 mobile
  inventory and domain dialog stayed within the viewport, with contained table scrolling.
- `task doctor` and `task local:test` passed. Cloud-specific race tests, static analysis,
  and PostgreSQL regressions additionally cover identical intent names in two projects,
  project-bound observations and scoped backup ownership/archive names. These local
  isolation checks are distinct from the live AWS effects below.

## Completed live evidence

| Path                     | Observed result                                                                                                                                                                         |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Public issuer            | Anonymous HTTPS discovery/JWKS and actual AWS web-identity exchanges; no signing keys or personal AWS credentials placed in workloads                                                   |
| Separate providers       | Exact S3/IAM/Route53 Kubernetes subjects and roles verified through actual STS account identity                                                                                         |
| Create/connect           | Two managed private/versioned/encrypted stores and one Observe-only supplied store; actual S3 owner/region/security metadata checked                                                    |
| Partial effects          | Missing S3 acceleration metadata permission initially blocked readiness after creation; narrow permission repair recovered the same bucket and unchanged versioned canary               |
| IAM creation             | Narrow name-only GetRole lookup repaired creation without widening role mutation beyond the consumer path/boundary                                                                      |
| Adoption                 | Connected fixture removed from the CFN template with Retain first; old review rejected, current inventory-bound review accepted; unmanaged tags and data preserved                      |
| Real CI                  | Private example PR #3 built/published multi-platform images in GitHub Actions and submitted its attested release through the existing thin Action; successful run 40737879650 attempt 2 |
| Component data           | Real web workload used its projected identity to put/get its assigned prefix; AWS denied out-of-prefix write and list                                                                   |
| DNS/TLS                  | Duplicate claim rejected; foreign A record left untouched; DNS-01 certificate issued; public DNS and HTTPS SNI health request succeeded with certificate verification enabled           |
| Shared grants            | Two stores worked through one exact web identity; removing one denied that store while preserving its sibling                                                                           |
| Preview removal          | Real PR closure removed its namespace, owned DNS and component grants; already-issued component credentials were denied while stores/archives remained                                  |
| Provider upgrade         | Pinned community S3 2.8.1 baseline → 2.9.0 runtime, with identical canary version and unchanged unmanaged tag after the corrected profile                                               |
| Provider revocation      | Explicit AWS deny blocked the running provider and already-issued provider credentials; recovery followed removal of the temporary deny                                                 |
| Controller/chart removal | All provider deployments stopped and the Crossplane chart was uninstalled; retained canary version, unmanaged tag and old archive objects remained                                      |
| Backup                   | Pinned CNPG/Barman source identity, completed base and scheduled backups, continuous archiving and actual S3 WAL keys observed                                                          |
| Restore identity         | Selected archive list allowed; write and out-of-prefix list denied; another Kubernetes subject could not assume the reader role                                                         |
| Restored data            | Separate CNPG sandbox recovered base data plus a marker committed after the latest base backup and then confirmed archived before restore                                               |

The first restore fixture switched WAL within the same implicit transaction as its
insert, so the commit had not reached the archived segment. That attempt recovered base
data only and is not counted as WAL proof. The corrected test committed the marker in
one request, switched WAL in another, confirmed the exact segment archived, then recreated
only the disposable restore sandbox/PVC. Recovery uses CNPG's default replay of all
available WAL; no recovery-target workaround was introduced.

The first CI attempt hit the development proxy during a console restart (HTTP 502).
Attempt 2 passed. Cloud binding changes subsequently selected immutable deployments
with the same admitted image provenance. Local Host/SNI routing to loopback proves
DNS/certificate integration, not public application reachability.

## Corrections established by live tests

An initial bucket Update policy removed an unmanaged tag even though the renderer omitted
tags. Bucket identity now omits Update; adopted bucket identity is Observe-only, while
four separate resources manage the approved security/versioning fields. The storage
provisioner also lacks bucket deletion and tag-writing IAM permissions. Repeated live
upgrade/retention checks preserved the tag and exact data version; actual attempts to
write tags, delete storage or read object data were denied.

Moving to archive generation 2 exposed asynchronous CNPG plugin RBAC propagation. The
agent now waits for exact ObjectStore get/status permissions and current archive access
before creating a base backup. A failed attempt is visible as Failed with Retry; the
retry completed and generation 2's WAL keys were observed. Its writer was denied list
and write against generation 1. The restore reader retained only its selected generation
1 and was denied generation 2. Restored application rows remained unchanged.

The first provider-revocation probe did not observe denial within its two-minute bound.
The completed check used AWS's documented older-session deny and a longer bounded wait;
the running provider reported actual GetBucketVersioning AccessDenied and the retained
SDK session was denied. This establishes observed revocation, not an instantaneous SLA.

Hosted CI initially failed existing workload/registry acceptance checks because pods
could not schedule. A subsequent instrumented run recorded `DiskPressure=True` and
the `node.kubernetes.io/disk-pressure:NoSchedule` taint during the CLI build, while
memory/PID pressure stayed false and the node stayed Ready. The runtime job now frees
only the unused Android SDK at `/usr/local/lib/android/sdk` on a verified disposable
GitHub-hosted Linux runner. Capacity monitoring remains in CI, and the local cluster's
10% disk reserve and workload tolerations are unchanged.

## Cleanup and scope

Retention was proved before the separately approved destructive fixture cleanup. Verified
cleanup removed four exact S3 buckets and all 53 versions/markers, seven IAM roles, the
dedicated permissions boundary and OIDC provider, both CloudFormation stacks, and the
dedicated child zone/delegation. All 21 unrelated parent-zone records were unchanged.
The isolated local M5 cluster and its synthetic source/restore storage were then removed.
The private example PR was closed without merging. Raw target inventories and delete
verification remain in owner-controlled private installation storage.

The optional profile does not add AWS/Crossplane to core installation, allocate application
PostgreSQL databases, expose cloud administration through existing MCP grants, or ship a
general remote capability installer. General PostgreSQL/onboarding/control-plane recovery
remains M6; broader controller lifecycle, hosted operation and additional provider/registry
profiles retain their own gates. Only the recorded versions and bounded development
workflow are accepted; production availability and public workload reachability are not claimed.

## Primary references

- [AWS JavaScript default credential chain](https://docs.aws.amazon.com/sdk-for-javascript/v3/developer-guide/setting-credentials-node.html)
- [AWS resource account conditions](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_condition-keys.html)
- [cert-manager Route53 web identity](https://cert-manager.io/docs/configuration/acme/dns01/route53/)
- [CNPG 1.30.1 recovery contract](https://github.com/cloudnative-pg/cloudnative-pg/blob/v1.30.1/docs/src/recovery.md)
- [Community provider 2.9.0 API source](https://github.com/crossplane-contrib/provider-upjet-aws/tree/v2.9.0)

- [AWS role-session revocation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_revoke-sessions.html)
- [Kubernetes node-pressure eviction](https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/)
- [GitHub-hosted runner capacity](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
- [Official Ubuntu 24.04 runner SDK locations](https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md)
