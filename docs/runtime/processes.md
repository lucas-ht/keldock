# Experimental runtime processes

This is a developer profile with local accounts, enrolled agents and delegated MCP reads and nonproduction operations.
Use [the disposable acceptance environment](local-development.md) to reproduce its
tested behavior. See [authentication and recovery](authentication.md) and the
[console guide](console.md). [GitHub App delivery](github-delivery.md) has an experimental
integration; general installation remains a later gate. These commands imply no production deployment.

## Commands

```sh
mise exec -- go build -o .local/keldock ./cmd/keldock
.local/keldock validate --file examples/multi-service/keldock.yaml
.local/keldock keygen --key-id build-key --private-key .local/build-key.pem --public-key .local/build-key.json
.local/keldock inspect --config .local/agent.json
.local/keldock serve --config .local/control.json --migrate
.local/keldock bootstrap --config .local/control.json --email operator@example.com --output .local/invitation.json
.local/keldock enroll --config .local/agent.json --cluster local-cluster --token-file .local/enrollment.token
.local/keldock agent --config .local/agent.json
```

Optional AWS commands (`cloud plan`, `cloud runtime`, `enroll --cloud`, `cloud-agent`)
have a [separate operator guide](cloud/README.md). Private `cloud` policies enable the API
profile; the cloud agent has its own credential, kubeconfig/context and `policies` list.
It receives no workload registrar credential. No cloud profile is enabled by default.

Run serve and agent as separate processes. `--migrate` explicitly applies checksummed
control-plane database migrations; the agent never migrates that database. Key generation
and attestation outputs refuse to overwrite existing files. Keep the private signing key
in trusted CI and copy only its public-key map to the operator's repository policy.

## Private configuration

Create configuration and credential files with mode 0600 outside Git. The local fixture
creates .local with mode 0700. Supply an independent random webhook secret of at least
32 bytes. Accounts, sessions and enrolled agent credentials are issued through their
separate lifecycles; there are no static operatorTokenFile or agentTokenFile settings.

Control configuration has these fields:

| Field                                   | Purpose                                                                                  |
| --------------------------------------- | ---------------------------------------------------------------------------------------- |
| databaseUrl                             | Operator-supplied PostgreSQL connection; local fixture value is in .local/runtime.json   |
| listen                                  | HTTP listen address behind operator-owned HTTPS; local testing uses loopback             |
| publicUrl                               | Canonical external HTTPS console/API origin; CSRF, OAuth and resource audience use this  |
| organizationId / clusterId              | Fixed installation ownership and placement                                               |
| domain                                  | Operator-controlled parent domain for stable staging/preview hostnames                   |
| registryPush / registryPull             | Same OCI repository path, with host/cluster routing aliases where needed                 |
| registryUsername / registryPasswordFile | Optional bundle publisher credential, separate from cluster readers                      |
| githubCredentialsFile                   | Owner-only App credential destination; enables GitHub setup, selection and feedback      |
| webhookSecretFile                       | Private GitHub webhook signature secret                                                  |
| githubIssuer                            | `https://token.actions.githubusercontent.com`; loopback issuer requires development mode |
| githubAudience                          | Instance-specific Actions OIDC audience                                                  |
| development / registryPlainHttp         | Explicit development exceptions; plain registry HTTP requires development mode           |
| projects                                | Operator-approved repository policies below                                              |

Each project contains id, name, repositoryId, ownerId, workflowPath, pushRef and
provenanceKeys. The workflow path is under .github/workflows; pushRef names the allowed
branch. provenanceKeys maps operator-approved key IDs to base64 Ed25519 public keys,
as produced by keygen. GitHub repository and owner IDs protect against silent transfers;
unsupported reusable-workflow and fork-submission profiles fail closed.

The separate agent configuration contains apiUrl, credentialFile, kubeconfig, context,
registryCredentialsFile, registryPull, registryPlainHttp and development. A supplied kubeconfig requires an
explicit context; an omitted kubeconfig selects in-cluster identity. API HTTP is allowed
only for an explicitly selected loopback development endpoint. The agent never follows
API redirects and receives no database or human session credential. The one-time
enrollment action writes its durable identity to credentialFile; the running agent
rotates it atomically. Inspect uses this same
configuration to read dependency compatibility without contacting the product API.

Both GitHub issuer and audience may be omitted for account/console-only operation;
release submission then fails closed with ci_not_configured. Supplying just one is
invalid. Project repository policies remain operator-owned configuration.

## Release submission and outcomes

Trusted CI produces a complete SubmitRelease JSON contract with the application,
every component's immutable digest, and exact source/workflow/run identity. It signs:

```sh
.local/keldock attest --private-key .local/build-key.pem --key-id build-key --input .local/unsigned-release.json --output .local/signed-release.json
```

CI submits that JSON to POST /v1/releases with its instance-audience OIDC bearer and a
stable Idempotency-Key. A 202 response contains release/environment/deployment/operation
IDs, desired generation and statusUrl. Repeating the same request/key returns the same
receipt; reusing a key with different input conflicts. Human, MCP and agent credentials cannot
bypass CI admission. The [thin Action](github-delivery.md) consumes these same APIs.

GET statusUrl accepts the authorized project CI identity, human session or scoped MCP grant. It
returns operation, observation, observationAgeSeconds and stale. Acceptance is not
readiness; missing/old observations stay visibly stale. Ready requires the complete
selected artifact and actual workloads/images. A client that stops waiting does not
cancel work. POST statusUrl/cancel is a human operator action with CSRF and retains
completed effects. POST statusUrl/retry creates a new generation using the admitted
release, checks expected generation and requires a stable Idempotency-Key.

Signed pull_request webhooks go to POST /v1/github/webhooks. Open/synchronize events
establish same-repository head state; closure persists teardown intent. Replays, older
events and late CI cannot reopen a closed preview. A strictly newer explicit reopen can
permit a new release. The local suite exercises these routes with a loopback JWKS issuer
and signed webhook fixtures; live App/Actions/GHCR results are recorded separately in
[M3 validation](../validation/m3-github-delivery.md).

## Preview and assistant operations

Project entries accept an operator-owned previews profile with bounded lifetime and
capacity. HTTP and MCP reuse admitted releases rather than accepting arbitrary images.
See [preview policy, receipts and skill workflows](agent-workflows.md) for the supported
configuration and operation contracts.
