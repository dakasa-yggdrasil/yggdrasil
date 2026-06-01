# Command reference

Every command, subcommand, and flag the `yggdrasil` CLI exposes, derived
directly from `cmd/yggdrasil/*` and `internal/*`. Where the code and a flag
default disagree with older prose, the code wins.

Global shape:

```
yggdrasil <command> [subcommand] [positionals] [flags]
yggdrasil <command> --help        # per-command flags
yggdrasil help                    # the full command tree
```

Most server-facing commands accept `--server <url>` and `--token <bearer>` to
override the active context for a single invocation. Otherwise they resolve the
server + token from `~/.yggdrasil/config.yaml` (see
[CONFIGURATION.md](CONFIGURATION.md)).

## Command index

| Command | Purpose |
|---|---|
| [`init`](#init) | Bootstrap a self-hosted stack, or attach to an existing core |
| [`login`](#login) | Exchange credentials for a session token; save a context |
| [`status`](#status) | Show the active context + a server health check |
| [`version`](#version) | Print the CLI version |
| [`apply`](#apply) | Create a new manifest version from a file |
| [`get`](#get) | List or fetch manifests |
| [`describe`](#describe) | Print one manifest as YAML |
| [`logs`](#logs) | Stream a workflow run's steps |
| [`deploy`](#deploy) | Apply a control_plane manifest + run the deploy workflow |
| [`install`](#install) | Quickstart-install an integration (GitHub / OCI) |
| [`auth`](#auth) | Manage OAuth/OIDC providers |
| [`collaborator`](#collaborator) | Workforce lifecycle |
| [`new`](#new) | Scaffold an integration or surface from a template |
| [`integrations`](#integrations-workspace-dev) | Workspace-dev: manage integration submodules |
| [`surfaces`](#surfaces-workspace-dev) | Workspace-dev: manage surface submodules |

---

## `init`

Bring up a full self-hosted stack with one command, or attach to an existing
`yggdrasil-core`. See [USAGE.md](USAGE.md#2-bootstrap-a-stack--yggdrasil-init)
for the bootstrap sequence.

```
yggdrasil init [--dir <path>]        # standalone docker-compose stack
yggdrasil init --server <url>        # attach to an existing core (skips compose)
```

| Flag | Default | Description |
|---|---|---|
| `--dir <path>` | `./yggdrasil` | Target directory for the compose stack. |
| `--server <url>` | _(empty)_ | Attach to an existing core; skips docker compose. |
| `--admin-username <slug>` | `admin` | First admin slug (login identifier). |
| `--admin-password <value>` | _(random)_ | First admin password. Empty = random 24-char, printed once at the end. |
| `--admin-email <email>` | _(empty)_ | First admin email (optional). |
| `--admin-display-name <name>` | `Admin` | First admin display name. |
| `--core-image <image>` | `ghcr.io/dakasa-yggdrasil/yggdrasil-core:latest` | yggdrasil-core container image. |
| `--port <int>` | `9080` | Host port for the core HTTP listener. |
| `--context <name>` | _(derived)_ | Context name in `~/.yggdrasil/config.yaml` (`local` standalone, or host-derived for `--server`). |
| `--yes` | `false` | Skip confirmation prompts. |

Standalone mode requires `docker` + the compose v2 plugin. It starts Postgres,
the core, and the `integration-kubernetes` + `integration-schema-migrations`
adapters; RabbitMQ is opt-in (`amqp` profile). It then logs in, saves a context,
and seeds two `integration_instance` manifests in namespace `global`.

---

## `login`

Exchange a username/password for a session token (`POST /api/v1/auth/login`) and
save it under a named context.

```
yggdrasil login --server <url> --username <slug> [--password <value>]
```

| Flag | Default | Description |
|---|---|---|
| `--server <url>` | _(prompt)_ | Core base URL (e.g. `http://localhost:9080`). |
| `--username <slug>` | _(prompt)_ | Login identifier (collaborator slug). |
| `--password <value>` | _(prompt, hidden)_ | Password. Omit to be prompted. |
| `--context <name>` | _(derived from host)_ | Context name to store under. |
| `--non-interactive` | `false` | Fail instead of prompting for missing values. |

Missing `--server` / `--username` / `--password` are prompted interactively
unless `--non-interactive` is set, in which case they are required.

---

## `status`

Print the active context and probe `/healthz` + `/readyz` (5s timeout).

```
yggdrasil status
```

| Flag | Description |
|---|---|
| `--server <url>` | Override the context server URL. |
| `--token <bearer>` | Override the context bearer token. |

Exits non-zero when the server is unreachable or not ready.

---

## `version`

```
yggdrasil version          # also: yggdrasil --version
```

Prints `yggdrasil <version> (<short-sha>)`. Release binaries embed the tag via
ldflags; source builds report `dev` plus the VCS revision when available. Takes
no flags.

---

## `apply`

Create a new version of any manifest kind from a file. The document needs
`kind`, `metadata` (with `name`), and `spec`. Multiple `---`-separated documents
are applied in order. POSTs to `/api/v1/manifests?kind=<kind>`.

```
yggdrasil apply -f my-workflow.yaml
yggdrasil apply -f - < bundle.yaml
```

| Flag | Default | Description |
|---|---|---|
| `-f <file>` | _(required)_ | Manifest file (YAML or JSON). Use `-` for stdin. |
| `--server <url>` | _(context)_ | Override the active context's server URL. |
| `--token <bearer>` | _(context)_ | Override the active context's bearer token. |

Prints `created <kind> <namespace>/<name> (version N)` per document.

---

## `get`

List manifests of a kind, or fetch one by name.

```
yggdrasil get <kind>                 # list all of a kind
yggdrasil get <kind> <name>          # fetch one by name
yggdrasil get <kind> -n <namespace>  # scope to a namespace
```

| Flag | Default | Description |
|---|---|---|
| `-n`, `--namespace <ns>` | _(all)_ | Filter by namespace. |
| `-o <format>` | `table` | Output format: `table` \| `yaml` \| `json`. |
| `--active-only` | `false` | Only manifests whose `metadata.active` is true. |
| `--server <url>` | _(context)_ | Override the context server URL. |
| `--token <bearer>` | _(context)_ | Override the context bearer token. |

Table columns: `KIND  NAMESPACE  NAME  VERSION  ACTIVE`.

---

## `describe`

Fetch one manifest and print the full document as YAML. Errors if the name is
ambiguous across namespaces (use `-n`).

```
yggdrasil describe <kind> <name> [-n <namespace>]
```

| Flag | Description |
|---|---|
| `-n`, `--namespace <ns>` | Namespace (required when ambiguous). |
| `--server <url>` | Override the context server URL. |
| `--token <bearer>` | Override the context bearer token. |

---

## `logs`

Stream a workflow run's step transitions until terminal. Polls
`GET /api/v1/workflow_runs/<id>`.

```
yggdrasil logs <run-id>
```

| Flag | Default | Description |
|---|---|---|
| `-f` | `true` | Follow the run until terminal. `-f=false` prints current state once. |
| `--interval <int>` | `2` | Polling interval in seconds (only with `-f`). |
| `--timeout <int>` | `600` | Give up after N seconds. |
| `--server <url>` | _(context)_ | Override the context server URL. |
| `--token <bearer>` | _(context)_ | Override the context bearer token. |

Terminal states are `succeeded`, `failed`, `cancelled`. A `failed` run makes the
command exit non-zero.

---

## `deploy`

Apply a `control_plane` manifest and dispatch the seeded
`yggdrasil-deploy-control-plane` workflow against it, printing per-step progress.
Only one target exists today.

```
yggdrasil deploy control-plane -f control-plane.yaml
yggdrasil deploy control-plane -f cp.yaml --no-run
```

| Flag | Default | Description |
|---|---|---|
| `-f <file>` | _(required)_ | `control_plane` manifest (YAML/JSON; `-` for stdin). |
| `--no-run` | `false` | Apply the manifest but skip the workflow run. |
| `--kubernetes-instance <name>` | `yggdrasil-core-kubernetes` | `integration_instance` used by the deploy workflow. |
| `--schema-migrations-instance <name>` | `yggdrasil-core-schema-migrations` | `integration_instance` used by the deploy workflow. |
| `--instance-namespace <ns>` | `global` | Namespace of the instance objects. |
| `--server <url>` | _(context)_ | Override the context server URL. |
| `--token <bearer>` | _(context)_ | Override the context bearer token. |

The manifest must declare `kind: control_plane` — `apply` is the right tool for
any other kind.

---

## `install`

Quickstart-install an integration on a remote `yggdrasil-core`. Fetches the
repo's `yggdrasil-quickstart.yaml`, collects the provider's inputs, and POSTs to
`/api/v1/integrations/install`.

```
yggdrasil install <repo_ref> [--provider id] [--input k=v ...] [--dry-run] [--non-interactive]
```

### `repo_ref` forms

| Form | Meaning |
|---|---|
| `owner/repo` | GitHub: default branch + `yggdrasil-quickstart.yaml` |
| `owner/repo@v1.2.3` | GitHub: pinned ref |
| `owner/repo:custom/path.yaml` | GitHub: custom manifest path |
| `owner/repo@ref:path` | GitHub: both |
| `oci://ghcr.io/owner/repo` | OCI: latest tag |
| `oci://ghcr.io/owner/repo:v1.2.3` | OCI: specific tag |

### Flags

| Flag | Default | Description |
|---|---|---|
| `--server <url>` | `$YGGDRASIL_URL` | Core base URL. |
| `--token <bearer>` | `$YGGDRASIL_WORKFLOW_RUN_TOKEN` | Bearer token for the install endpoint. |
| `--provider <id>` | _(picker)_ | Provider id to install; skips the picker. |
| `--input <k=v>` | _(repeatable)_ | Pre-fill an input. May appear multiple times. |
| `--dry-run` | `false` | Ask the server to compile the workflow without dispatching it. |
| `--non-interactive` | `false` | Skip the TUI; read inputs from `--input` / defaults (CI mode). |
| `--yes` | `false` | Skip the final confirmation (implied by `--non-interactive`). |

For OCI refs the CLI passes the fetched manifest bytes inline so the server
need not re-authenticate to the registry. Auth env: `$YGGDRASIL_GITHUB_TOKEN`
(→ `$GITHUB_TOKEN`) for private GitHub + GHCR; `$OCI_USERNAME` / `$OCI_PASSWORD`
override OCI credentials.

---

## `auth`

Manage authentication providers. Today the only group is `provider`.

```
yggdrasil auth provider list
yggdrasil auth provider get <name>
yggdrasil auth provider apply -f <file>
yggdrasil auth provider delete <name>
```

| Subcommand | Flags | Description |
|---|---|---|
| `provider list` | `--server`, `--token` | List configured providers (`GET /api/v1/auth/providers`). |
| `provider get <name>` | — | Fetch one provider as YAML. |
| `provider apply -f <file>` | `-f` (required), `--server`, `--token` | Register/update a provider from YAML/JSON; prints the third-party login URL. |
| `provider delete <name>` | — | Remove a provider. |

The applied YAML must contain a `name`. `apply` POSTs to
`/api/v1/auth/providers`.

---

## `collaborator`

Workforce lifecycle. Output is JSON. All verbs resolve the active context (no
per-verb `--server`/`--token`). `<id>` accepts a collaborator id or slug for
`get`; other verbs take an id.

```
yggdrasil collaborator <verb> [options]
yggdrasil collaborator help
```

### Lifecycle

| Verb | Required flags | Optional flags |
|---|---|---|
| `create` | `--slug`, `--display-name` | `--email`, `--status` (default `active`), `--start-date`, `--role`, `--manager`, `--team` |
| `get <id\|slug>` | — | — |
| `list` | — | `--status` |
| `update <id>` | at least one of `--display-name`, `--email`, `--status` | — |
| `offboard <id>` | `--reason` (`voluntary\|involuntary\|contract-end\|deceased`) | `--end-date`, `--notice-days` |
| `suspend <id>` | — | `--reason` |
| `unsuspend <id>` | — | `--reason` |
| `re-onboard <id>` | — | `--start-date`, `--role` |

### Field changes

| Verb | Required flags | Optional flags |
|---|---|---|
| `role-change <id>` | `--new-role` | — |
| `team-add <id>` | `--team` | `--role-in-team` |
| `team-remove <id>` | `--team` | — |
| `attribute-set <id>` | `--key` | `--value`, `--type` (`string\|number\|bool\|json`) |
| `manager-change <id>` | — | `--new-manager` (empty to clear) |

### Absence & audit

| Verb | Required flags | Optional flags |
|---|---|---|
| `absence-start <id>` | `--type` (`vacation\|leave-medical\|leave-parental\|leave-sabbatical`) | `--from`, `--to`, `--days` |
| `absence-end <id>` | — | `--absence-event-id`, `--actual-end` |
| `lifecycle-events <id>` | — | `--type`, `--limit` (default `100`) |
| `provider-state <id>` | — | — |

Backed by `/api/v1/collaborators*` endpoints on the core.

---

## `new`

Scaffold a new plugin from an official template
(`dakasa-yggdrasil/<kind>-template`). Shallow-clones, rewrites the module path +
project name, strips template git history, and inits a fresh repo.

```
yggdrasil new integration <name> [flags]
yggdrasil new surface <name> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--module <path>` | `github.com/<owner>/<kind>-<name>` | Go module path. |
| `--owner <org>` | _(empty)_ | GitHub owner for the default module + install hint. |
| `--dir <path>` | `./<kind>-<name>` | Target directory. |
| `--template <ref>` | `dakasa-yggdrasil/<kind>-template` | Override the template repo (`owner/repo[@ref]`). |
| `--no-git-init` | `false` | Skip `git init` in the new directory. |

`<name>` must be lowercase kebab-case starting with a letter. Requires `git` on
`PATH`. The scaffold compiles + tests cleanly on creation.

---

## `integrations` (workspace dev)

Contributor-only. Works **only** from a checkout of the Yggdrasil monorepo (it
needs `catalog/integrations.json` to find the workspace root). Manages
integration submodules in the local tree — not the adopter install flow (use
[`install`](#install) for that).

```
yggdrasil integrations list
yggdrasil integrations install <slug>
yggdrasil integrations remove <slug>
yggdrasil integrations tui
yggdrasil integrations installed
```

| Subcommand | Description |
|---|---|
| `list` | Table of installable integrations from the workspace catalog. |
| `install <slug>` | Add the integration repo as a submodule under `integrations/`. |
| `remove <slug>` | Remove an installed integration submodule. |
| `tui` | Open the Bubble Tea integration manager. |
| `installed` | Print docker-compose files for installed integrations. |

---

## `surfaces` (workspace dev)

Contributor-only, same monorepo requirement. Manages UI/edge surface submodules.

```
yggdrasil surfaces list
yggdrasil surfaces active
yggdrasil surfaces install <name>
yggdrasil surfaces remove <name>
yggdrasil surfaces installed
yggdrasil surfaces scaffold <name> [module]
yggdrasil surfaces activate <name>
yggdrasil surfaces deactivate <name>
```

| Subcommand | Description |
|---|---|
| `list` | Cataloged surfaces with install/active status. |
| `active` | Surfaces currently active in the product runtime. |
| `install <name>` | Add a surface as a submodule under `surfaces/`. |
| `remove <name>` | Remove an installed surface submodule. |
| `installed` | Print docker-compose files for installed active surfaces. |
| `scaffold <name> [module]` | Scaffold a new surface from the reference template. |
| `activate <name>` | Mark a surface active in the product runtime. |
| `deactivate <name>` | Remove a surface from the active runtime. |

---

See [CONFIGURATION.md](CONFIGURATION.md) for env vars and the config file, and
[USAGE.md](USAGE.md) for the end-to-end walk-through.
