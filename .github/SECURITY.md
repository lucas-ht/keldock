# Security

## Current status

Keldock is a private development repository with no supported runtime release. No
production security, tenant-isolation, availability, or recovery guarantee is made by
the bootstrap. Supported versions and an operational reporting route must be documented
before the first public release.

## Reporting

Do not put credentials, exploit details, or customer data in public issues. Use GitHub
private vulnerability reporting if enabled for this repository. Otherwise contact the
maintainer through their GitHub profile to arrange a private reporting channel before
sharing sensitive details. This document does not claim that private reporting is enabled.

## Development boundaries

- Keep credentials, kubeconfigs, private installation inputs, plans, and state out of Git.
- CI validates source; it does not acquire cluster/cloud authority or deploy workloads.
- Use scoped, renewable identities and explicit resource ownership in runtime designs.
- Record actual negative authorization tests, dependency versions, and recovery evidence
  before advertising a capability.

See the [agent guide](../AGENTS.md) for operational rules.
