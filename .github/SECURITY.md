# Security

## Current status

Keldock's 0.6.0-alpha.1 candidate passed developer and assisted fresh-target
acceptance. It remains unpublished and limited to the documented development
profile. No production security, hostile-tenant isolation, availability or complete
disaster-recovery guarantee is made. There is no maintained public release yet.

## Reporting

Do not put credentials, exploit details or customer data in public issues. The
planned reporting route is [GitHub private vulnerability reporting](https://github.com/lucas-ht/keldock/security/advisories/new).
It is not currently enabled for this private candidate. Do not send a report until
that route is confirmed operational.

Before public alpha publication, the maintainer must enable the feature, verify
the reporting form and confirm repository security-alert notifications. GitHub
documents this feature for public repositories and makes notifications dependent
on the maintainer's watch settings; see [its reporting configuration guide](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository).
No private email address, response SLA or paid reporting service is implied.

## Development boundaries

- Keep credentials, kubeconfigs, private installation inputs, plans, and state out of Git.
- CI validates source; it does not acquire cluster/cloud authority or deploy workloads.
- Use scoped, renewable identities and explicit resource ownership in runtime designs.
- Record actual negative authorization tests, dependency versions, and recovery evidence
  before advertising a capability.

See the [agent guide](../AGENTS.md) for operational rules.
