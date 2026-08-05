# Claude Code Context: yggdrasil (CLI + monorepo)

Start with `AGENTS.md` for the rules-of-engagement summary. This file
expands the context for Claude-style assistants.

## What this repo is

Two things in one repo:

1. **`yggdrasil` — the official CLI** (`cmd/yggdrasil/`). Talks to a
   running `yggdrasil-core` HTTP API: `init`, `apply`, `get`, `describe`,
   `install`, `deploy`, `logs`, `login`, `status`, `version`, `new`,
   `collaborator`.
2. **The Yggdrasil product monorepo** — submodules pulling in the
   service + reference surfaces so the whole stack can be inspected in
   one tree.

Repo: `github.com/dakasa-yggdrasil/yggdrasil` (open source, Apache 2.0).

## Stack

- Go 1.25.
- Charmbracelet UI stack (`bubbletea`, `huh`, `lipgloss`) — the TUI
  for `yggdrasil init` / `login`.
- `sigs.k8s.io/yaml` for manifest parsing in apply/get/describe.
- **No** server code lives in this repo directly. All logic with
  authority is in `services/yggdrasil-core` (submodule).

## Repo layout

```
cmd/yggdrasil/                  # CLI entrypoint + subcommands
internal/corecli/               # Code shared across CLI subcommands (HTTP client, config, …)
internal/initcli/               # `yggdrasil init` bootstrap orchestration
internal/integrations/          # `yggdrasil install` catalog logic (incl. OCI refs)
internal/quickstartcli/         # `yggdrasil-quickstart.yaml` parsing/rendering
internal/scaffoldcli/           # `yggdrasil new integration` scaffolder
internal/surfaces/              # Helpers for surface commands
dev/compose/                    # docker-compose bootstrap shipped by `yggdrasil init`
templates/surface-go/           # Surface scaffold
services/yggdrasil-core/        # submodule → dakasa-yggdrasil/yggdrasil-core
surfaces/yggdrasil-console/     # submodule → dakasa-yggdrasil/surface-console
surfaces/yggdrasil-auth-surface/# submodule → dakasa-yggdrasil/surface-auth
integrations/                   # README only (catalog stub)
catalog/                        # Local integration catalog manifests
docs/                           # User-facing CLI docs
scripts/                        # Dev tooling
```

## Important: this repo is NOT the deployable source

- The cluster runs **`dakasa-yggdrasil/yggdrasil-core`** directly. The
  `services/yggdrasil-core/` submodule here is **observe-only** — the
  Yggdrasil repository binding points elsewhere.
- Surface submodules likewise: code changes go in their standalone
  repos; the bumped submodule pointer in this monorepo is informational.
- The `yggdrasil` CLI binary is the only thing this repo builds and
  releases on its own (`release.yml`).

## CI / image flow

- `.github/workflows/release.yml` — `vX.Y.Z` tag triggers a CLI release
  (multi-OS binaries, no container image — the CLI is a single static
  binary delivered via GitHub Releases).
- `.github/workflows/ci.yml` — go test + lint + smoke.
- `.github/workflows/emit-deploy-event.yml` — POSTs deploy event so
  yggdrasil-core can react (same soft-skip pattern as core).
- `.github/workflows/deploy.yml` — placeholder for the monorepo-level
  deploy event (not used as primary deploy path).
- `.github/workflows/incident-escalation.yml` + `postmortem.yml` —
  Heimdall-driven ops automation.

The CLI also ships `yggdrasil-quickstart.yaml` consumers, including OCI
ref support (`oci://...`, commit `6da5dfe`) so adopters can install
catalogued integrations by container ref.

## Validation

```bash
task arch:check     # architecture invariants
task config         # config schema validation
task smoke          # full product smoke (manual: see ci.yml)
task build:images   # release/runtime packaging changes
go test ./...
```

## Mandatory contracts

- **Business authority stays in `services/yggdrasil-core`.** Don't
  re-implement workflow/manifest logic in the CLI — call the HTTP API.
- **Surfaces stay thin.** Submodule edges (`surfaces/yggdrasil-console`,
  `surfaces/yggdrasil-auth-surface`) are presentation only; they MUST
  NOT become alternative sources of truth.
- **No direct imports across services/surfaces.** Use the published
  HTTP API + manifests.
- **Submodule pointers** — bump intentionally with
  `git submodule update --remote <path>` + commit; never make in-tree
  edits to submodule contents from this repo.
- **CLI is API-compatible with the core minor version it shipped
  against.** When the core adds a new endpoint (e.g. DELETE
  `/api/v1/manifests/{id}`, commit `5cbdecb` in core), wire the CLI
  flag in the corresponding subcommand and pin the core minor in the
  CLI release notes.

## Where things live

- Apply/get/describe HTTP plumbing → `internal/corecli/`
- TUI bootstrap (`yggdrasil init`) → `internal/initcli/` + `dev/compose/`
- Catalog install (incl. OCI refs) → `internal/integrations/`
- `yggdrasil new integration` scaffolder → `internal/scaffoldcli/`
  (clones `dakasa-yggdrasil/integration-template`, rewrites module paths)

## Docs freshness (AI-reconciled, stamp-gated) — mandatory

**Before you open a PR or merge, update every doc your change makes stale.** If you touched an
integration contract, an op signature, a capability, a surface, an event, or any behavior a doc
describes, the matching doc under `docs/` (ADRs, contracts, the map) and any affected README or
doc-comment must move in the same change. A doc that still describes the old behavior is a false
witness the next reader trusts.

**Prove it with the stamp.** Each docs tree carries `docs/AI_DOCS_FRESHNESS.md` with
`verified_at_commit` (the commit an AI or agent-assisted human last reconciled those docs at).
When you reconcile the docs, bump that field to your branch tip. On arrival, if the stamp is
behind the code you are about to touch, reconcile those docs FIRST, before trusting them.

**CI is the safety net.** `.github/workflows/docs-freshness.yml` runs on every PR: a cheap gate
checks whether real code changed and the stamp was NOT bumped; only then does an AI reconcile
the stale docs, commit ONLY the doc files back to the PR branch, and bump the stamp. If your
agent already reconciled and stamped, the gate skips the AI (the economy path). The reconciler
never weakens a doc to match a bug and never touches source.

**Prose docs carry a human-feel layer.** Machine-precise docs (service contracts, schemas, the
freshness stamp) come first, but whenever you write or update a PROSE doc (a README, an ADR, an
architecture narrative, a runbook, a connectivity view), make it legible at a glance: a mermaid
diagram for anything with a flow, a graph, or a sequence; real markdown tables, never ASCII art;
short sections over walls of text. IA first, human feel beside it.
