# Tested profile and limits

The candidate supports one organization, one existing cluster and a small trusted
team. Release publication and an independent operator walkthrough are still pending.
Use the [validation index](../../validation/README.md) for the evidence boundary.

| Component                                   | Candidate pin                         |
| ------------------------------------------- | ------------------------------------- |
| Kubernetes / K3s                            | 1.35.5 / v1.35.5+k3s1                 |
| Flux CLI / source and kustomize controllers | 2.10.0 / 1.10.1                       |
| Helm                                        | 4.3.1                                 |
| Control-plane PostgreSQL                    | 18.4                                  |
| Bundle registry                             | Distribution 3.0.0                    |
| Application CNPG / PostgreSQL               | 1.30.1 / 17.7 standard trixie         |
| Optional cert-manager / Barman Cloud        | 1.21.3 / 0.15.1                       |
| Runtime / console build                     | Go 1.27.3 / Node 24.22.1, pnpm 11.1.2 |

Images use immutable digests; upstream manifests have locked checksums. The release
renderer also pins Barman's base64-encoded sidecar image. Supplied versions outside
this profile require their own compatibility evidence. Installing controller manifests
does not silently adopt an existing operator or grant module installation through MCP.
Runtime images were exercised on Linux/arm64 under Docker Desktop. The kit's Linux
and macOS amd64/arm64 CLI binaries are cross-builds; other runtime architectures
need their own cluster acceptance before being advertised as tested.

The core candidate passed delivery on an independent node constrained to **2 CPU
and 4 GiB RAM**, including K3s, Traefik, Flux, API/worker, console, control-plane
PostgreSQL, bundle registry and an active five-component synthetic application.
Four samples over 30 seconds peaked at 1,756 MiB; its cgroup lifetime peak was
2,200 MiB. This is the tested development floor, not a production load benchmark
or a promise that the optional PostgreSQL/backup profile fits the same budget.
Chart requests and container limits are configuration, not measurements.
Reserve persistent capacity
for retained data, registry graphs and restore copies; repeated previews do not free
retained database quota or disk automatically.

The alpha does not promise HA, zero downtime, hostile tenant isolation, arbitrary
ingress/storage compatibility, managed backups, automatic archive garbage collection,
cross-cluster recovery or PostgreSQL major upgrades. CNPG integration is an optional
development profile. Application migrations may have irreversible effects; unchanged
image promotion and release rollback do not reverse those effects.

Build, test, cache and image publication stay in customer CI. GHCR private workload
pulls use the verified M3 reader path; AWS/ECR, Docker Hub and private Harbor-style
workload integrations remain deferred. A hosted control plane, wider module catalogue,
multi-cluster operation and hosting remain later evidence gates.

Contributions follow [the repository guide](../../../.github/CONTRIBUTING.md).
Report vulnerabilities using [the security reporting procedure](../../../.github/SECURITY.md).
No public availability or production recovery guarantee follows from the AGPLv3 license.
