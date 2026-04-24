<div align="center">

# `yggdrasil`

**Official CLI for the [Yggdrasil](https://github.com/dakasa-yggdrasil/yggdrasil-core) control plane**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](go.mod)
[![Release](https://img.shields.io/github/v/release/dakasa-yggdrasil/yggdrasil?include_prereleases&sort=semver)](https://github.com/dakasa-yggdrasil/yggdrasil/releases)

*One command bootstrap. kubectl-style manifest operations. Integration catalog installs in one line.*

[Install](#install) · [Commands](#commands) · [Docs](https://github.com/dakasa-yggdrasil/yggdrasil-core/tree/main/docs)

</div>

---

## What it does

`yggdrasil` is the terminal interface to a self-hosted
[Yggdrasil control plane](https://github.com/dakasa-yggdrasil/yggdrasil-core).
Think of it as `kubectl` for workflows, integrations, and the rest of the
Yggdrasil manifest catalog — plus a one-command bootstrap that brings up the
entire stack on a fresh host.

```sh
yggdrasil init                                                # Boot Postgres + RabbitMQ + core + admin in one command
yggdrasil install dakasa-yggdrasil/integration-kubernetes     # Install an integration from the catalog
yggdrasil apply -f my-workflow.yaml                           # Apply any manifest (YAML or JSON)
yggdrasil get workflow -n prod                                # List manifests (table / yaml / json)
yggdrasil logs <run-id>                                       # Stream a workflow run's step results
```

## Install

### From release (recommended)

Grab a prebuilt binary for your OS/arch from the
[latest release](https://github.com/dakasa-yggdrasil/yggdrasil/releases/latest),
then:

```sh
# macOS (Apple Silicon example)
curl -L https://github.com/dakasa-yggdrasil/yggdrasil/releases/latest/download/yggdrasil_$(uname -s)_$(uname -m).tar.gz | tar xz
sudo mv yggdrasil /usr/local/bin/
yggdrasil version
```

### From source

```sh
go install github.com/dakasa-yggdrasil/yggdrasil/cmd/yggdrasil@latest
```

### Via Homebrew / Scoop

> Package manager taps are on the roadmap. Track
> [issue #N](https://github.com/dakasa-yggdrasil/yggdrasil/issues) for progress.

## Quick start

```sh
# Boot a fresh self-hosted stack on this machine (needs Docker or colima)
yggdrasil init

# See what you've got
yggdrasil status

# Install the Kubernetes integration so workflows can apply K8s manifests
yggdrasil install dakasa-yggdrasil/integration-kubernetes --provider kubernetes

# Apply a workflow you wrote
yggdrasil apply -f my-workflow.yaml

# Watch it run
yggdrasil logs <run-id>
```

A more thorough walk-through lives in the core repo's
[getting-started guide](https://github.com/dakasa-yggdrasil/yggdrasil-core/blob/main/docs/getting-started.md).

## Commands

### Bootstrap and connection

| Command | What it does |
|---|---|
| `yggdrasil init` | Bring up a full self-hosted stack via docker-compose (Postgres + RabbitMQ + core + seeded admin + catalog). Use `--server <url>` to attach to an existing yggdrasil-core instead. |
| `yggdrasil login --server <url> --username <slug>` | Exchange credentials for a session token and save a named context to `~/.yggdrasil/config.yaml`. |
| `yggdrasil status` | Print the active context + a health check against the server. |
| `yggdrasil version` | Print the CLI version (and commit SHA if built from source). |

### Manifest operations

| Command | What it does |
|---|---|
| `yggdrasil apply -f <file>` | Create a new version of any manifest kind. YAML or JSON. Use `-` for stdin. |
| `yggdrasil get <kind> [<name>]` | List (or fetch one) manifests. Supports `-n <namespace>` and `-o table\|yaml\|json`. |
| `yggdrasil describe <kind> <name>` | Full YAML dump of one manifest. |
| `yggdrasil logs <run-id>` | Stream a workflow run's step transitions until terminal. |

### Integration catalog

| Command | What it does |
|---|---|
| `yggdrasil install <repo_ref>` | Install an integration from a Github repo that carries a `yggdrasil-quickstart.yaml`. Interactive TUI for required inputs; `--non-interactive` and `--input k=v` for CI. |

### Build your own plugin

| Command | What it does |
|---|---|
| `yggdrasil new integration <name> [--owner <org>]` | Scaffold a new integration adapter from the official template. Shallow-clones, renames the Go module, inits a fresh git repo. `go test ./...` passes on the spot. |
| `yggdrasil new surface <name> [--owner <org>]` | Same for surfaces (UI/auth edges). |

Full walkthrough:
[extending.md](https://github.com/dakasa-yggdrasil/yggdrasil-core/blob/main/docs/extending.md).

### Auth providers (OAuth/OIDC)

| Command | What it does |
|---|---|
| `yggdrasil auth provider list` | List configured OAuth/OIDC providers. |
| `yggdrasil auth provider apply -f <file>` | Register or update a provider (see [auth-providers/](https://github.com/dakasa-yggdrasil/yggdrasil-core/tree/main/docs/auth-providers) for GitHub + Google templates). |
| `yggdrasil auth provider get <name>` | Fetch one provider's config. |
| `yggdrasil auth provider delete <name>` | Remove a provider. |

### Contributor helpers (monorepo dev)

The following commands only work when run from a checkout of the Yggdrasil
monorepo and are intended for contributors — not adopters:

| Command | What it does |
|---|---|
| `yggdrasil integrations list|install|tui|installed` | Clone an integration repo into the local workspace for dev. |
| `yggdrasil surfaces list|install|scaffold|activate` | Manage UI surfaces in the local workspace. |

## Configuration

Config lives at `~/.yggdrasil/config.yaml` (kubectl-style multi-context).
Structure:

```yaml
current_context: local
contexts:
  local:
    server: http://localhost:9080
    token: ys_...
    collaborator: admin
  prod:
    server: https://yggdrasil.example.com
    token: ys_...
    collaborator: ops-bot
```

Switch contexts for one command without editing the file:

```sh
YGGDRASIL_CONTEXT=prod yggdrasil get workflow
```

Point at a different config file entirely:

```sh
YGGDRASIL_CONFIG=/etc/yggdrasil/ci.yaml yggdrasil apply -f workflow.yaml
```

Full reference:
[docs/cli.md](https://github.com/dakasa-yggdrasil/yggdrasil-core/blob/main/docs/cli.md).

## Shell completion

> Completion scripts are on the roadmap. For now, `yggdrasil help` prints
> the full command tree.

## Contributing

This CLI is the primary touchpoint adopters have with Yggdrasil, so PRs
that improve ergonomics, error messages, or integration coverage are
especially welcome.

- Run the test suite: `go test ./...`
- Build locally: `go build -o yggdrasil ./cmd/yggdrasil`
- Try a release snapshot: `goreleaser release --snapshot --clean --skip=publish`

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

<div align="center">

Part of the [Yggdrasil](https://github.com/dakasa-yggdrasil/yggdrasil-core) project · [Report an issue](https://github.com/dakasa-yggdrasil/yggdrasil/issues)

</div>
