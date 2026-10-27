# Experimental runtime processes

This is a developer profile for the first runtime, with static installation credentials.
Use [the disposable acceptance environment](local-development.md) to reproduce its
tested behavior. General installation, human login, enrollment and GitHub App onboarding
are later gates; no production deployment is implied by these commands.

## Commands

```sh
mise exec -- go build -o .local/keldock ./cmd/keldock
.local/keldock validate --file examples/multi-service/keldock.yaml
.local/keldock keygen --key-id build-key --private-key .local/build-key.pem --public-key .local/build-key.json
.local/keldock inspect --config .local/agent.json
.local/keldock serve --config .local/control.json --migrate
.local/keldock agent --config .local/agent.json
```

Run serve and agent as separate processes. `--migrate` explicitly applies checksummed
control-plane database migrations; the agent never migrates that database. Key generation
and attestation outputs refuse to overwrite existing files. Keep the private signing key
in trusted CI and copy only its public-key map to the operator's repository policy.

## Private configuration

Create configuration and credential files with mode 0600 outside Git. The local fixture
creates .local with mode 0700. Supply unique random credentials of at least 32 bytes for
operator, agent and webhook secret; the operator and agent credentials must differ.

Control configuration has these fields:

| Field                                                  | Purpose                                                                                  |
| ------------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| databaseUrl                                            | Operator-supplied PostgreSQL connection; local fixture value is in .local/runtime.json   |
| listen                                                 | HTTP listen address behind operator-owned HTTPS; local testing uses loopback             |
| organizationId / clusterId                             | Fixed installation ownership and placement                                               |
| domain                                                 | Operator-controlled parent domain for stable staging/preview hostnames                   |
| registryPush / registryPull                            | Same OCI repository path, with host/cluster routing aliases where needed                 |
| registryUsername / registryPasswordFile                | Optional publisher credential; private pull behavior remains unvalidated                 |
| operatorTokenFile / agentTokenFile / webhookSecretFile | Separate private credential files                                                        |
| githubIssuer                                           | `https://token.actions.githubusercontent.com`; loopback issuer requires development mode |
| githubAudience                                         | Instance-specific Actions OIDC audience                                                  |
| development / registryPlainHttp                        | Explicit development exceptions; plain registry HTTP requires development mode           |
| projects                                               | Operator-approved repository policies below                                              |

Each project contains id, name, repositoryId, ownerId, workflowPath, pushRef and
provenanceKeys. The workflow path is under .github/workflows; pushRef names the allowed
branch. provenanceKeys maps operator-approved key IDs to base64 Ed25519 public keys,
as produced by keygen. GitHub repository and owner IDs protect against silent transfers;
unsupported reusable-workflow and fork-submission profiles fail closed.

The separate agent configuration contains apiUrl, tokenFile, kubeconfig, context,
registryPull, registryPlainHttp and development. A supplied kubeconfig requires an
explicit context; an omitted kubeconfig selects in-cluster identity. API HTTP is allowed
only for an explicitly selected loopback development endpoint. The agent never follows
API redirects and receives no database or operator credential. Inspect uses this same
configuration to read dependency compatibility without contacting the product API.

## Release submission and outcomes

Trusted CI produces a complete SubmitRelease JSON contract with the application,
every component's immutable digest, and exact source/workflow/run identity. It signs:

```sh
.local/keldock attest --private-key .local/build-key.pem --key-id build-key --input .local/unsigned-release.json --output .local/signed-release.json
```

CI submits that JSON to POST /v1/releases with its instance-audience OIDC bearer and a
stable Idempotency-Key. A 202 response contains release/environment/deployment/operation
IDs, desired generation and statusUrl. Repeating the same request/key returns the same
receipt; reusing a key with different input conflicts. Operator and agent tokens cannot
bypass CI admission. The thin GitHub Action and live feedback remain M3 work.

GET statusUrl accepts the authorized project CI identity or development operator. It
returns operation, observation, observationAgeSeconds and stale. Acceptance is not
readiness; missing/old observations stay visibly stale. Ready requires the complete
selected artifact and actual workloads/images. A client that stops waiting does not
cancel work. POST statusUrl/cancel is an operator action and retains completed effects.

Signed pull_request webhooks go to POST /v1/github/webhooks. Open/synchronize events
establish same-repository head state; closure persists teardown intent. Replays, older
events and late CI cannot reopen a closed preview. A strictly newer explicit reopen can
permit a new release. The local suite exercises these routes with a loopback JWKS issuer
and signed webhook fixtures; no live GitHub integration is claimed.
