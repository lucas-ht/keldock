# 0001: FOSS and customer control

Status: accepted, 2026-10-27.

## Context

Keldock should make application operations accessible to individuals and small teams,
while supporting larger organizations. A future commercial offering should operate the
product without making a paid service mandatory for self-hosting.

## Decision

Retain the repository's AGPLv3 license and build a complete supported self-hosted core.
Include APIs, authorization, audit, recovery, MCP and first-party skills in that core.
Require no hosted account, subscription validation or enterprise identity provider.
Keep customer clusters, application data and infrastructure ownership explicit.

Offer paid operation of the same core later: hosted control plane first, then potentially
managed customer infrastructure and provider-operated compute. Validate demand, costs,
isolation and recovery separately. Commercial packaging and prices remain undecided.

Third-party capabilities retain their licenses. Document those boundaries and maintain
a fully FOSS installation path. No license change is made by this bootstrap.

## Consequences

Portability, independent bootstrap and recovery are product requirements. Core platform
features cannot quietly become hosted-only dependencies. Small-team adoption must be
measured because Kubernetes and operational responsibility still carry real cost.

Customer control, MCP and self-hosting alone are not unique market claims. Differentiate
through the complete combination and a demonstrated usable workflow. See
[product direction](../product/direction.md).
