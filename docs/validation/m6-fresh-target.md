# M6 fresh-target assisted acceptance

Status: passed on 2026-10-29 for runtime source `d97ecbd`. Lucas explicitly
delegated the fresh-target journey to the agent, retaining account sign-in and
security-sensitive approvals. This is assisted acceptance; it does not establish
independent human usability or public availability.

## Target and source boundary

A separate operator workspace, private example repository, installation-owned GitHub
App, CI signing key and retained storage were used on `k3d-keldock-walkthrough`.
Its node has 4 CPUs/6 GiB, K3s 1.35.5, default Traefik and local-path. Core delivery
ran before the optional CNPG/Barman controllers were installed. The original M3
application was a protected, separately checked target. No M6 AWS apply ran.

The first ec70bc4 kit was checksummed and installed without maintainer fixture
credentials or synthetic admission. Corrections were packaged from clean committed
source, including all four CLI targets, source, chart, pinned dependencies and skills.
The d97ecbd kit had 563 verified members; its packaged Linux arm64 CLI was identical
to the binary placed in the runtime image. Image references are immutable.
These are local candidates; publication remains a separate decision.

## Observed results

| Gate                   | Fresh-target evidence                                                                                                                                                                                                                                                                                                    |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Core onboarding        | Canonical HTTPS, user-created password, registrar enrollment, connected setup and real five-component customer CI delivery passed before optional database controllers were installed.                                                                                                                                   |
| GitHub delivery        | A new App selected only the private example repository. CI built/published private GHCR digests and signed complete releases; Actions received no Kubernetes credentials. No application Flux manifests were authored.                                                                                                   |
| Stateless preview      | A draft preview was created, updated and closed. A late pull-patch Job failure was corrected and the failed CI attempt rerun successfully. Closure removed its workload namespace.                                                                                                                                       |
| MCP and skills         | The retained local OAuth/PKCE client discovered ten tools with only walkthrough read consent. Cross-project access and deployment writes were denied. Both bundled skills used the same read boundary.                                                                                                                   |
| PostgreSQL             | An explicitly approved development policy provided four retained slots with 1 GiB local-path per allocation. The documented full-policy enrollment command succeeded after its CLI parsing fix.                                                                                                                          |
| Application data       | Staging ran all five selected components on the admitted digest. The app role had no superuser/createdb/createrole privileges. Worker and scheduled SQL events were observed. Unbound web and preview-to-staging database access were denied.                                                                            |
| Data preview           | PostgreSQL PR #2 passed customer CI with its own empty database and zero staging markers. Closure removed workloads but preserved the database/PVC UIDs and its own committed marker. The retained allocation still consumed the second of four slots.                                                                   |
| Native backup          | Separate exact-prefix writer/read-only restore identities, pinned Secret UIDs and trusted HTTPS protected one exact source UID. A real base completed; a later committed marker was archived through WAL.                                                                                                                |
| Isolated restore       | A separate namespace/cluster replayed that post-base marker exactly once, verified through SQL. It had no application binding; the source UID was unchanged.                                                                                                                                                             |
| Control-plane capture  | Installed/operator CLIs verified independent pre-preview and current snapshots. The latter contained 38 checksummed files and 12 referenced immutable OCI graphs, including workload images and private installation material.                                                                                           |
| Fenced recovery        | Both named empty databases restored with PostgreSQL 18 tools and refused server startup while fenced. Stale review rejected the newer surviving preview checkpoint. The current review matched three workload checkpoints, two retained allocations, deleted previews and the unchanged active admin.                    |
| Recovered authority    | Recovered browser sessions, MCP grants, pending enrollment/invitation grants and all agent purposes were revoked. Old registrar/database bearers returned 403 and old MCP returned 401. Offline admin recovery ran only in the permanently fenced stale database; the resumed password was unchanged.                    |
| Artifact recovery      | Six bundle manifests were imported into an independent initially empty registry and verified at their original content digests. Captured private workload graphs were not republished.                                                                                                                                   |
| Resume                 | Only the reviewed current database resumed. Explicit registrar/database enrollment and fresh read-only MCP consent converged the same staging generation 2 and fresh database/WAL observations. A read-only write attempt was denied without changing the generation.                                                    |
| Availability           | All 1,800 one-second samples spanning control-plane recovery returned application HTTP 200. This is local acceptance evidence, not an availability guarantee.                                                                                                                                                            |
| Uninstall retention    | After a verified new snapshot and purpose/MCP revocation, only the reviewed Helm release was uninstalled. All eight PVCs, four application/data namespaces, four checkpoint ConfigMaps, three database source UIDs and three archive Secret UIDs remained. All three data markers and baseline archive objects survived. |
| Supplied dependencies  | Nine reviewed operator-owned objects reused the retained PostgreSQL/registry volumes. Reinstallation used the matching kit with both embedded dependencies disabled. Its Helm manifest contained no dependency StatefulSets. New explicit enrollment/consent restored fresh health; old MCP authority remained denied.   |
| Lifecycle availability | All 743 one-second application samples spanning uninstall/reinstall and verification returned HTTP 200. Additional archive WAL objects appeared while every baseline object retained its size/content identity.                                                                                                          |

Live customer-CI evidence is in the private walkthrough repository: core run
40849179097, stateless preview update 40851485083 attempt 2, native staging
40857270981 and PostgreSQL preview 40859729697. Private repository access is
required; these are distinct from synthetic local admission fixtures.

Local `task doctor` passed. The complete post-capacity-recovery `task local:test`
passed: runtime 723.126 seconds, access 12.524, GitHub 6.807 and local cloud contracts
5.718. [Hosted CI for d97ecbd](https://github.com/lucas-ht/keldock/actions/runs/40858463732)
also passed doctor and runtime. Cloud-contract fixtures are not AWS validation.

## Assistance, failures and limits

The initial stateless workload source was selected from the matching kit's bundled
fixture because the guide did not point to it directly; the walkthrough now does.
The standalone MCP client initially used an obsolete handshake and was corrected
against the pinned SDK/current protocol. An operator helper initially transferred
an empty reader file; strict parsing rejected it before capture. Its transfer was
then checked byte-for-byte. None of these steps bypassed admission or authorization.

The journey corrected unchanged-existing-project PostgreSQL enablement, modern MCP
browser preflight, immutable release Job pull-patch handling, full provisioner
enrollment configuration parsing and current selected Job/prerequisite health.
Real Flux regressions passed, including a partial-registration source barrier.

Shared Docker disk pressure caused failed runs and temporary evictions on both
active local targets. The reserve was never weakened. Authorized cache cleanup and
an explicit allowlist of ten unreferenced image builds restored headroom; all Docker
container IDs and volumes were preserved. The original M3 app recovered with its
original commit. Failed runs are not counted as passes.

The account-less HTTPS tunnel had intermittent transport failures and was later
removed by its provider. A replacement origin/App routing was explicitly approved;
a loopback buffering proxy stabilized the operator transport without changing TLS
validation. This is an assisted test transport, not a production hosting profile.
Operators need their own stable HTTPS/DNS and independent encrypted snapshot storage.

Snapshots and recovery copies remain owner-only local acceptance material. The
journey does not prove off-machine recovery, total cluster/volume loss, HA, arbitrary
registry/provider compatibility, hostile tenant isolation or independent usability.
Public security reporting and publication remain separate reviewed gates.
