# 0007: Optional AWS ownership, identity and retained resources

Status: implementation in progress. M5 cloud acceptance is not established by this decision.

## Scope and ownership

Keep AWS absent from the default installation. The optional profile adds project-scoped
CloudConnections, private object storage, domain claims and backup destinations/bindings.
An installation operator supplies approved account/region/profile/identity configuration;
application input cannot submit IAM policies, provider configurations or controller code.
The existing MCP operation grant does not acquire cloud administration or data deletion.

Keep four writers distinct:

| Owner                            | Objects                                                                                                        |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Operator bootstrap               | Public OIDC discovery, trust roots, provisioning roles/boundaries, delegated DNS test zone, pinned controllers |
| Cloud agent                      | Approved typed projections, provider configuration and independently retained inventory                        |
| Crossplane providers             | Only cloud fields explicitly delegated by those projections                                                    |
| Workload registrar/Flux and CNPG | Application workloads and PostgreSQL internals, respectively                                                   |

Use a separate cloud-agent identity and namespace authority. The public API receives no
AWS access-key pair. The workload agent does not inherit provider authority. Resource
records and cloud projections remain outside disposable preview namespace ownership.

The bootstrap owns any supplied DNS zone and issuer infrastructure. Crossplane owns
specific records/resources, not the parent's lifecycle. Terraform, CloudFormation,
operator GitOps and Crossplane must not concurrently manage the same external fields.

## Supported first profile

Use Crossplane 2.4.3 and the Apache-2.0 community AWS providers 2.9.0. Community artifacts
are published under xpkg.upbound.io/crossplane-contrib, separately from commercial
Upbound builds. Pin package digests; verify actual controller images and CRD behavior
in the disposable cluster before treating the profile as supported.

Use namespace-scoped AWS managed resources. Metadata conditions alone do not prove
external changes or usable access. Verify the account, external identity, selected
configuration and actual allowed/denied effects. Use managementPolicies explicitly:
connect is Observe-only; managed resources omit Delete by default. Never enable
forceDestroy for storage. A resource cannot be adopted merely because its name exists.

New managed buckets use S3's account-regional namespace and stable account/region/resource
names. Existing bucket access requires an exact operator grant and verified owner/region;
it does not require adopting the bucket. Support private, owner-enforced, versioned,
SSE-S3 storage first. Additional encryption/namespace/provider profiles need their own
permission and lifecycle evidence.

Consumer bindings resolve exact environment/component service accounts, object prefixes
and IAM permissions. Do not accept role ARNs, namespaces or raw policies in application
binding input. Preserve the existing network isolation contract while enabling the
approved AWS data path. Identity, networking and resource usability are separate checks.

DNS claims stay within an approved parent domain and hosted zone. Preserve a unique
ownership marker and reject another writer's claim. Record convergence and certificate
readiness separately. DNS record cleanup must verify ownership; it cannot delete the
hosted zone or another application's records.

## Short-lived trust outside EKS

Do not assume that Kubernetes has a public issuer. A self-managed cluster must publish
its own public discovery/JWKS documents through an operator-approved HTTPS endpoint.
Publish public keys only; keep signing keys in the cluster. Match issuer, audience and
exact namespace/service-account subjects in AWS trust policies. Do not use wildcard
subjects or put the operator's personal AWS credentials in controllers or workloads.

Give S3, IAM and Route53 providers distinct service-account/role pairs. Bound IAM-created
consumer roles by path and a required permissions boundary, and prevent modification
of provisioning roles or that boundary. DNS solvers receive only delegated TXT scope.
Connection revocation must distinguish rejected new intent, stopped reconciliation,
revoked AWS authority and retained external resources; a disabled UI flag proves none
of the latter by itself.

Use an isolated local M5 cluster for public-issuer validation. Preserve the earlier
M3/M4 cluster, approved workloads and state. Live acceptance needs separately approved
AWS account, region, resource prefix and DNS delegation. Inspect the exact target and
prepare reviewable IAM/resource changes before requesting apply approval.

## Retention, adoption and recovery

Connecting, binding, adopting, retaining, disconnecting and destroying are different
operations. Adoption needs current external identity/field inventory, an explicitly
released previous writer, a review bound to the exact revision, and preserved data.
Do not silently replace an existing resource after a failed observation. Retain resources
on preview deletion, controller removal, disconnected agents and interrupted clients.

Use CloudNativePG 1.30.1 with Barman Cloud Plugin 0.15.1 for the optional backup
integration, and cert-manager 1.21.3 for certificate dependencies. Supplied installations
retain their owner. Bootstrap installation is explicit; this does not introduce M8's
general remote module installer or complete M6's alpha packaging.

Separate archive generations and writer/restore identities. Scheduled/base-backup and
WAL evidence must remain visible. A restore uses an isolated destination and must prove
actual recovered fixture data, not just a Ready controller condition. The restored
cluster cannot overwrite its source archive. M5's live backup test does not establish
production availability, general upgrades or control-plane recovery.

## Evidence gates and references

Extend task doctor and task local:test with policy, transactional authority, receipts,
revocation, wrong-project/account/region denials, stale review and retained inventory.
The optional runtime must exercise real Crossplane APIs; local fixtures cannot replace
live AWS create/connect/adopt, scoped access, DNS conflict/TLS, partial effects,
revocation, retention and isolated backup restore. Record each evidence class separately.

Primary sources consulted for this implementation:

- [Crossplane 2.4 managed resources](https://docs.crossplane.io/v2.4/managed-resources/managed-resources/): management policies and external ownership.
- [Community versus commercial AWS providers](https://www.upbound.io/blog/an-update-on-upbounds-official-providers) and [AWS provider 2.9.0 source](https://github.com/crossplane-contrib/provider-upjet-aws/tree/v2.9.0): distribution, license and actual API fields.
- [AWS self-managed Kubernetes IRSA](https://anywhere.eks.amazonaws.com/docs/getting-started/optional/irsa/): discovery, public signing keys and exact workload trust.
- [S3 general-purpose namespaces and naming](https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucketnamingrules.html): account-regional names and reserved aliases.
- [Barman Cloud Plugin usage](https://cloudnative-pg.io/plugin-barman-cloud/docs/usage/): ObjectStore, WAL, backup and separate restore/archiving configuration.
