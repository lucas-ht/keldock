# 0008: Self-hosted alpha and PostgreSQL allocation

Status: implementation in progress, 2026-10-28. Acceptance is tracked in M6;
this decision alone does not establish a supported installation.

The alpha packages the existing Go API/worker, outbound registrar and Next.js
console. The public processes receive no Kubernetes credentials. Optional
PostgreSQL provisioning runs under a separate enrolled identity. PostgreSQL for
control-plane state remains independently installable and recoverable.

Application definitions request named PostgreSQL resources from operator-approved
profiles. Each environment receives its own CNPG cluster, database, non-superuser
owner, retained namespace and PVCs. A new preview starts empty. Only declared
components receive the binding Secret and database network access. Neither a
release rollback nor preview deletion deletes or restores database contents.
Retained allocations count against capacity until an operator removes them.

The first integration uses the already tested CNPG 1.30.1/PostgreSQL 17.7 and
Barman Cloud 0.15.1 pins. Reusing a supplied operator requires compatibility and
ownership checks; installing one is an explicit bootstrap operation. A supplied
S3-compatible destination and separate writer/restore identities suffice. AWS
and Crossplane are optional. Restore creates another cluster and never replaces
the source or silently attaches recovered data to an application.

Control-plane snapshots include database state, referenced private installation
files and an artifact inventory. They require private storage and operator
encryption off-machine. Restore targets an empty, independent database and stays
fenced until its state is compared with the surviving cluster's durable journal.
Older generations, conflicting owners and removed-preview tombstones block
resumption. The supported recovery procedure never deletes a newer workload to
make a restored snapshot agree.

Packaging acceptance must install from the documented inputs on an independent
disposable cluster, exercise core delivery without CNPG, then the optional data
profile, upgrade, recovery and uninstall retention. Record resource measurements,
versions, failures and exact commands. Release archives and images must be
reviewable before requesting publication. A separate operator's walkthrough
remains a distinct evidence gate and cannot be inferred from automated tests.

Primary references used during implementation:

- [CNPG bootstrap and unprivileged application owners](https://cloudnative-pg.io/docs/1.30/bootstrap/)
- [CNPG role ownership, credentials and reclaim policy](https://cloudnative-pg.io/docs/1.30/declarative_role_management/)
- [CNPG recovery creates a new cluster](https://cloudnative-pg.io/docs/1.30/recovery/)
- [PostgreSQL consistent logical backup](https://www.postgresql.org/docs/18/app-pgdump.html)
- [Flux tenant impersonation](https://fluxcd.io/flux/installation/configuration/multitenancy/)
