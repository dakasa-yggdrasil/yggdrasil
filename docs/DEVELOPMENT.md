# Development

Build, test, and release the `yggdrasil` CLI, plus the repo's layout and the
monorepo/submodule split. Verified against `Taskfile.yml`, `.goreleaser.yaml`,
`.github/workflows/`, and `go.mod`.

---

## Prerequisites

- **Go 1.25+** (`go.mod` declares `go 1.25.0`).
- **git** (required by `yggdrasil new` and the workspace-dev commands).
- For the standalone stack / smoke tests: **Docker** + the **compose v2** plugin.
- Optional: [Task](https://taskfile.dev) for the contributor workflows,
  [goreleaser](https://goreleaser.com) v2 for release dry-runs.

---

## Build & run

```sh
go build -o yggdrasil ./cmd/yggdrasil
./yggdrasil help

# or install onto your PATH
go install github.com/dakasa-yggdrasil/yggdrasil/cmd/yggdrasil@latest
```

The version string is injected at link time. A plain `go build` reports `dev`
(plus the VCS revision); release archives embed the tag via
`-ldflags "-X main.version=<tag>"`.

---

## Test

```sh
go test ./...            # all unit tests (cmd + internal packages)
task arch:check          # architecture-boundary check (./scripts/check-boundaries.sh)
```

CI (`.github/workflows/ci.yml`) runs `task arch:check` then `go test ./...` on
every push to `main` / `codex/**` and on PRs. The heavier `product`,
`installation-manager` jobs are `workflow_dispatch`-only (they spin up the full
stack via submodules).

Notable test files you'll touch when changing behavior:

- `cmd/yggdrasil/apply_test.go`, `collaborator_test.go` — CLI subcommand logic.
- `internal/corecli/config_test.go`, `collaborators_test.go` — config + client.
- `internal/quickstartcli/quickstartcli_test.go`, `oci_test.go` — install flow + OCI refs.
- `internal/scaffoldcli/scaffold_test.go` — `yggdrasil new`.
- `internal/initcli/initcli_test.go`, `seeds_test.go` — bootstrap + topology seeds.

---

## Release

Releases are produced by goreleaser, triggered by a `vX.Y.Z` tag (or manual
`workflow_dispatch`) via `.github/workflows/release.yml`.

```sh
# dry-run a release locally — builds the matrix, skips publishing
goreleaser release --snapshot --clean --skip=publish
```

The matrix (from `.goreleaser.yaml`): `linux`, `darwin`, `windows` × `amd64`,
`arm64` (Windows/arm64 excluded), `CGO_ENABLED=0`. Archives are named
`yggdrasil_<version>_<os>_<arch>.tar.gz` (Windows → `.zip`), with a
`checksums.txt`. There is **no container image** — the CLI ships as a single
static binary via GitHub Releases.

To cut a release:

```sh
git tag v1.2.3
git push origin v1.2.3      # triggers the release workflow
```

---

## Repo layout

This repository is two things: the CLI source, and a product monorepo that
pulls the server + reference surfaces in as submodules for inspection.

```
cmd/yggdrasil/                 # CLI entrypoint + one file per subcommand
internal/corecli/              # shared HTTP client, config, data shapes
internal/initcli/              # `yggdrasil init` bootstrap (compose assets embedded)
internal/integrations/         # workspace-dev `integrations` submodule manager
internal/quickstartcli/        # `yggdrasil install` (quickstart manifest, OCI, TUI)
internal/scaffoldcli/          # `yggdrasil new integration|surface` scaffolder
internal/surfaces/             # workspace-dev `surfaces` submodule manager
dev/compose/                   # contributor monorepo dev stack (infra.yml + .env)
templates/surface-go/          # reference surface scaffold
catalog/                       # local workspace catalog (integrations.json, surfaces.json)
scripts/                       # dev/ops shell tooling (smoke, boundaries, heimdall)
docs/                          # this docs suite + product specs/audits
services/yggdrasil-core/       # submodule → dakasa-yggdrasil/yggdrasil-core
surfaces/yggdrasil-console/    # submodule → dakasa-yggdrasil/surface-console
surfaces/yggdrasil-auth-surface/  # submodule → dakasa-yggdrasil/surface-auth
Taskfile.yml                   # contributor task runner
.goreleaser.yaml               # CLI release matrix
```

### What this repo builds vs. observes

- The **CLI binary** is the only artifact this repo builds and releases on its
  own (`release.yml`).
- `services/yggdrasil-core/` and the two `surfaces/*` are **submodules** — the
  cluster runs those standalone repos directly. Here they're observe-only:
  changes to server/surface code go in their own repos; the bumped submodule
  pointer in this monorepo is informational.
- **Business authority stays in `yggdrasil-core`.** Don't re-implement
  workflow/manifest logic in the CLI — call the HTTP API. Surfaces stay thin.

### Submodule notes

```sh
git submodule update --init --recursive       # hydrate submodules
git submodule update --remote <path>          # bump a pointer (then commit)
```

Never make in-tree edits to submodule contents from this repo. Bump pointers
intentionally and commit the bump separately.

> The `surfaces/yggdrasil-console` submodule pointer is frequently dirty in
> working copies (an unrelated console bump). Don't stage it alongside CLI/doc
> changes.

---

## Contributor stack (Task)

The `Taskfile.yml` drives the full monorepo dev stack (not the CLI's
`init`-generated standalone stack). It uses the repo-root `.env` /
`dev/compose/`:

```sh
task doctor        # check docker + compose + task are present
task env:init      # seed .env files from the .example templates
task up            # start the full local stack (Postgres, core, surfaces, ...)
task config        # validate/render the compose configuration
task smoke         # live smoke check against the local stack (needs `task up`)
task down          # stop the stack
```

Mandatory validation before a runtime/API change: `task arch:check`,
`task config`, and `task smoke` for runtime changes (mirrors `AGENTS.md`).

---

## Scaffolding

`yggdrasil new <kind> <name>` (see [COMMANDS.md](COMMANDS.md#new)) clones
`dakasa-yggdrasil/<kind>-template`, then applies two ordered find/replace
rewrites across every text file (binary files — those with a NUL byte in the
first 8 KiB — are skipped):

1. `github.com/dakasa-yggdrasil/<kind>-template` → your `--module`.
2. `<kind>-template` → `<kind>-<name>`.

It strips the template's `.git`, then `git init` + `git add -A` (unless
`--no-git-init`). The result compiles and `go test ./...` passes immediately —
the canonical shape other integrations/surfaces copy.

---

## Where things live

| Task | Code |
|---|---|
| HTTP client / config | `internal/corecli/client.go`, `config.go` |
| `init` bootstrap + compose assets | `internal/initcli/` (`assets/docker-compose.yml`, `env.template`) |
| Topology `integration_instance` seeds | `internal/initcli/seeds.go` |
| `install` quickstart + OCI | `internal/quickstartcli/` |
| `new` scaffolder | `internal/scaffoldcli/scaffold.go` |
| Collaborator client | `internal/corecli/collaborators.go` |

For the product server itself, see
[yggdrasil-core](https://github.com/dakasa-yggdrasil/yggdrasil-core).
