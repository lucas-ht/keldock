# AWS setup and trust

This extension has three independent authorities:

1. The API commits authorized intent and receipts; it receives no operator AWS keys.
2. The workload registrar installs generated workloads through namespaced Flux identities.
3. The separately enrolled cloud agent projects only approved cloud/database intent.
   Crossplane's S3, IAM and Route53 providers each have an exact service account and role.

Keep all installation JSON, credentials, kubeconfigs, public-issuer configuration and
review bundles outside Git, in owner-only files/directories. Specify the exact kubeconfig
and context; the cloud agent does not use an implicit Kubernetes context.

## Operator bootstrap

`keldock cloud plan --config /private/bootstrap.json --out /private/new-review`
prepares CloudFormation foundation/identity templates, a policy example and a plan hash.
It makes no AWS calls or applies. The bootstrap specification contains organization,
project, cluster and short installation IDs; account/region; parent hosted-zone ID and
domain; a strict child domain; the ingress IPv4 address; and the development flag.

Review the actual account, caller, region, DNS delegation, resource prefixes, IAM paths,
boundary, issuer and cost before a separately approved apply. The foundation owns the
child zone/delegation, an OIDC discovery bucket and a connected storage fixture. The
identity stack owns the OIDC provider, provisioning/solver roles and consumer boundary.
All are retained on stack deletion. Deleting the stacks alone does not clean them up.
The generated connected bucket is for acceptance; ordinary users can supply compatible
existing storage through explicit profile grants without adopting it.

For a self-managed Kubernetes cluster, configure its service-account issuer and publish
only its public discovery/JWKS documents through the reviewed HTTPS origin. Never publish
or extract signing private keys. Keep existing public keys available through signing-key
rotation until their issued tokens have expired. An issuer URL change is an operator
migration, not a connection edit.

Replace the generated child-zone placeholder with the actual output. Add approved public
IPv4 HTTPS egress ranges and any supplied database targets, then place matching `cloud`
(API) and `policies` (cloud-agent) arrays in the private process settings. The agent checks
its own local policy independently. These are trusted operator configuration, not request
fields or an application manifest. Configuration changes require a controlled restart;
account, region and identity references are immutable within a connection.

`keldock cloud runtime --config /private/policy.json --out /private/new-runtime.json`
renders the locked provider/runtime objects for operator inspection and application.
It does not install controllers. [The artifact lock](../../../internal/cloud/crossplane/controllers.lock.json)
contains the tested controller/chart/provider versions and digests.
`tooling/cloud/controllers.mjs` is restricted to the isolated local M5 acceptance cluster;
it is not an arbitrary-cluster installer. Supplied controllers keep their existing owner.

Enroll with `keldock enroll --cloud --config /private/cloud-agent.json --cluster <id>
--token-file /private/enrollment.token`, then run
`keldock cloud-agent --config /private/cloud-agent.json`. Enrollment and lease/rotation
credentials are independent from the workload registrar. Revoke one without silently
revoking or granting the other.

## IAM and network boundaries

Provider credentials use projected tokens and short-lived web-identity sessions. Match
issuer, audience and exact namespace/service-account subjects. No wildcard subjects,
node credentials, personal profiles or installation access keys reach applications.
IAM-created roles require the installation's consumer path and permissions boundary.
Provisioners cannot pass roles, edit their own trust roots or change that boundary.

Bindings receive a stable per-component role, the union of their active approved stores,
and environment/component object prefixes. S3 grants also constrain the resource account.
Backup writer and restore reader are separate identities. The reader is granted only
the explicitly selected restore generation. The storage provisioner checks
metadata and lists archive keys, but cannot read or write object contents.

Workload HTTPS egress is an explicit operator allowlist alongside default deny, cluster
DNS and declared application dependencies. Kubernetes NetworkPolicy does not supply an
FQDN firewall. Pinning one DNS answer is insufficient for rotating STS endpoints. The
live fixture used published regional S3 ranges plus the smallest published regional AWS
range containing the STS endpoints. That AWS range is not service-exclusive; IAM still
restricts actual AWS authority. Operators must review range changes; no automatic address
refresh, IPv6 egress or general internet access is implied.

Route53 providers are restricted to the supplied child zone. The certificate solver
uses a separate TXT-only role and explicit TokenRequest permission. Supply a compatible
ClusterIssuer before requesting domains; see the [cert-manager Route53 documentation](https://cert-manager.io/docs/configuration/acme/dns01/route53/).
The profile does not make a loopback/private ingress publicly reachable.

The cloud agent needs permission to submit Kubernetes SubjectAccessReviews for backup
readiness. These query the exact source service account's ObjectStore get/status-update
authority; the agent waits for CNPG's RBAC reconciliation instead of editing CNPG-owned
roles. Cloud-agent/API versions must be upgraded together in this development profile:
observations identify their project explicitly, and generated backup ownership/names
and archive prefixes include organization/project scope.
