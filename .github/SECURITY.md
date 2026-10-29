# Security

## Current status

Keldock's public 0.6.0-alpha.1 development alpha passed developer and assisted fresh-target
acceptance. It remains limited to the documented development
profile. No production security, hostile-tenant isolation, availability or complete
disaster-recovery guarantee is made. See the
[release and verification record](../docs/validation/m6-fresh-target.md#public-alpha-publication).

## Reporting

Do not put credentials, exploit details or customer data in public issues. The
reporting route is [GitHub private vulnerability reporting](https://github.com/lucas-ht/keldock/security/advisories/new).
It is enabled; the public entry point and maintainer security-alert subscription
were verified on 2026-10-29.

Maintainers must preserve the reporting route and its notification subscription.
GitHub documents the feature for public repositories and makes notifications dependent
on the maintainer's watch settings; see [its reporting configuration guide](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository).
No private email address, response SLA or paid reporting service is implied.

## Development boundaries

- Keep credentials, kubeconfigs, private installation inputs, plans, and state out of Git.
- CI validates source; it does not acquire cluster/cloud authority or deploy workloads.
- Use scoped, renewable identities and explicit resource ownership in runtime designs.
- Record actual negative authorization tests, dependency versions, and recovery evidence
  before advertising a capability.

See the [agent guide](../AGENTS.md) for operational rules.
