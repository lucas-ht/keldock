# 0004: First runtime profile

Status: accepted, 2026-10-27. Experimental core M1 profile; not a self-hosted alpha.

## Runtime and ownership

Use a modular Go binary with separate `serve` and `agent` commands. Serve runs the
HTTP API and PostgreSQL-backed publication worker. The agent initiates outbound API
requests and is the sole registrar for one cluster. These process boundaries preserve
the option of an external or hosted control plane without introducing hosted tenancy.

PostgreSQL 18.4 is the tested durable-state profile. Acceptance commits immutable inputs,
selection, dispatch, audit and receipt together. Workers use expiring leases and
[SKIP LOCKED](https://www.postgresql.org/docs/18/sql-select.html) to coordinate. Publication
can repeat after interruption and produces the same OCI digest. This is at-least-once
work execution: application jobs still need idempotent effects. Cancellation freezes
dispatch/reconciliation and retains completed effects; it does not roll them back.

An operator establishes the organization, repository IDs, project placement, approved
workflow, signing keys and preview domain. Startup refuses to reassign stored ownership.
Application configuration cannot select a cluster, namespace, issuer or administrative
identity. Resource/capability/enrollment types reserve domain boundaries; they do not
expose installation, allocation or enrollment APIs in this development runtime.

## Supported development dependencies

The disposable profile tests Kubernetes/K3s 1.35.5, Flux source/kustomize controllers
1.10.1, Traefik, PostgreSQL 18.4 and OCI distribution 3.0.0. Exact tool/image pins live
with the executable fixture. Flux uses scoped impersonation, no remote bases and no
cross-namespace references. OCI workload bundles carry no registration/RBAC authority.

`keldock inspect` reads supplied controller images, availability, served Flux APIs,
isolation flags and the Traefik ingress class. It does not apply objects or adopt a
controller. The local bootstrap explicitly owns its disposable cluster; this is not a
general installer for arbitrary existing clusters. Private-registry credential/pull
profiles require additional M2 evidence before being supported.

## Authentication and packaging

The development process profile uses distinct operator and cluster credentials from
owner-only files. The agent receives neither database access nor the operator token.
Changing/removing its configured credential and restarting the API rejects that agent;
the agent stops on rejection. Single-use enrollment, persisted revocation and rotation
remain M2 work, rather than being inferred from these static development credentials.

CI uses GitHub Actions OIDC plus the separately signed DSSE/SLSA release described in
the [contracts](../runtime/contracts.md). Signed PR events establish the current head
and closure state. The tested merge commit and PR head remain separate identities.
No build, test runner or application-image publisher is part of the control plane.

For human product access, select local administrator login with server-side sessions,
CSRF protection and project/environment grants before exposing UI/MCP writes. This
will work without enterprise SSO; the development operator token is not that login
implementation. Package the working commands now; ship an all-in-one Helm installation
with optional supplied dependencies at M6 after auth, recovery and upgrade acceptance.
Do not add an empty chart or an untested image as a substitute for that evidence.

## MCP and skills profile

Select MCP 2026-07-28 and the official Go SDK v1.8.0 for the first adapter. The
[pinned SDK's compatibility table](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
lists that protocol and stdio support. Start with stdio and an explicitly configured,
scoped product API credential. Stdio itself is not a new authorization authority; the
API must enforce the same grants as other clients. Remote MCP must additionally meet
the [MCP authorization profile](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).

This selects the integration target, not a shipped MCP dependency or compatibility
claim. Add the SDK with the working M2 adapter and test a real SDK client and assistant
host before advertising it. First-party skills follow the [Agent Skills format](https://agentskills.io/specification)
and working tools in M4. Neither profile requires an LLM provider or paid account.

## Evidence gates

The [local acceptance](../runtime/local-development.md) tests durable work, signed
identity/event fixtures, OCI/Flux delivery, real HTTP routing, multi-service previews,
failure/recovery, pruning and isolation. It also runs compiled server and agent processes.
Local OIDC/webhook fixtures do not close M3's live GitHub App/Actions/feedback gate.

M2 still needs product authentication, enrollment/revocation, UI, MCP reads and registry
access profiles. PostgreSQL capability/restore and optional AWS evidence retain their
separate M6/M5 gates. The core runtime does not install CNPG, Crossplane, Vault or Temporal.
