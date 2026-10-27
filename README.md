# Keldock

Keldock is a planned free and open-source control plane for deploying and operating
applications on customer-controlled infrastructure. It is intended for individuals,
small teams, and larger organizations, with shared APIs for people and AI agents.

## Status

This is a fresh repository bootstrap. Validation tooling exists; no product server,
agent, MCP endpoint, GitHub Action, runtime schema, or installable chart is implemented.

## Development

Use the versions in [.mise.toml](.mise.toml):

```sh
mise install
mise exec -- task bootstrap
mise exec -- task doctor
```

`task doctor` checks repository contracts, formatting, Markdown links, workflow syntax,
and validation-tool tests. It does not validate a Kubernetes deployment or cloud access.

See [contributing](.github/CONTRIBUTING.md), [security](.github/SECURITY.md), and the
[agent guide](AGENTS.md). The repository retains its [AGPLv3 license](LICENSE).
