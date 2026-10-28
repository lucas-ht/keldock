# Architecture decisions

These decisions capture accepted product boundaries. They do not claim implementation.
Update the architecture and affected acceptance gates when superseding a decision.

| Decision                                                                                   | Status                                                  |
| ------------------------------------------------------------------------------------------ | ------------------------------------------------------- |
| [0001: FOSS and customer control](0001-foss-and-customer-control.md)                       | Accepted                                                |
| [0002: State and reconciliation](0002-state-and-reconciliation.md)                         | Accepted; core runtime implemented                      |
| [0003: Delivery and staged scope](0003-delivery-and-staged-scope.md)                       | Accepted; core runtime implemented; later gates pending |
| [0004: First runtime profile](0004-first-runtime-profile.md)                               | Accepted; experimental development profile              |
| [0005: Shared authentication and remote MCP](0005-shared-authentication-and-remote-mcp.md) | Accepted; implemented experimental M2 profile           |
| [0006: Bounded nonproduction operations](0006-bounded-nonproduction-operations.md)         | Accepted; implemented experimental M4 profile           |
| [0007: Optional AWS profile](0007-optional-aws-profile.md)                                 | Implementation in progress; cloud acceptance pending    |

Unsettled implementation choices belong in [M1](../implementation-plan.md#m1-contracts-and-bounded-spikes).
Do not infer a runtime pin or a supported compatibility profile from an architectural
preference or an upstream documentation link.
