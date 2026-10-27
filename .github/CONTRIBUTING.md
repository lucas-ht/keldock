# Contributing to Keldock

Keldock is at repository-bootstrap stage. Prefer small changes that advance a complete
user workflow. Design documents are requirements, not claims of implemented behavior.
Start with the [architecture](../docs/architecture.md) and
[implementation plan](../docs/implementation-plan.md).

## Setup

Install [mise](https://mise.jdx.dev/getting-started.html), then use the pinned tools:

```sh
mise install
mise exec -- task bootstrap
mise exec -- task doctor
```

The equivalent commands with the pinned Node.js and pnpm already active are:

```sh
pnpm install --frozen-lockfile --ignore-scripts
pnpm check
```

Run `task format` before committing. `task doctor` includes formatting, Markdown
syntax/local links, repository/CI invariants, workflow lint, and validation-tool tests.
There is no runtime test command until a real deployment fixture exists.

## Changes

- Read [AGENTS.md](../AGENTS.md) and the owning design before editing.
- Keep product code and synthetic examples independent of personal installations.
- Update design and ownership decisions before changing lifecycle semantics.
- Pin dependencies and commit the corresponding lockfiles. Install with scripts
  disabled unless an implementation demonstrates why a specific script is needed.
- Add meaningful failure-path tests with behavior, not placeholders or copied assertions.
- Keep conventional commit and PR titles, for example `feat(agent): ...`.
- Report validation scope and remaining prerequisites. Formatting is not runtime proof.

The existing [AGPLv3 license](../LICENSE) applies. Publication, cloud deployment, and
ownership transfers are separate reviewed operations.
