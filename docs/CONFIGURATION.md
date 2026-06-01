# Configuration

How the CLI finds a server, authenticates, and which environment variables it
reads. All names and defaults below are verified against `internal/corecli/`,
`internal/quickstartcli/`, and `internal/initcli/`.

---

## The config file — `~/.yggdrasil/config.yaml`

`init` and `login` write a `kubectl`-style multi-context file. It holds, per
context, the server URL, the session token, and the collaborator slug.

```yaml
current_context: local
contexts:
  local:
    server: http://localhost:9080
    token: <session-token>
    collaborator: admin
  prod:
    server: https://yggdrasil.example.com
    token: <session-token>
    collaborator: ops-bot
```

### Fields

| Field | Scope | Description |
|---|---|---|
| `current_context` | top-level | Name of the active context (used when `YGGDRASIL_CONTEXT` is unset). |
| `contexts` | top-level | Map of context name → context. |
| `contexts.<name>.server` | context | Base URL of the `yggdrasil-core` (e.g. `http://localhost:9080`). |
| `contexts.<name>.token` | context | Bearer session token. **Secret.** Sent as `Authorization: Bearer`. |
| `contexts.<name>.collaborator` | context | Login slug for the saved session (informational; shown by `status`). |

### File location & permissions

- Default path: `$HOME/.yggdrasil/config.yaml`.
- Override the whole path with `YGGDRASIL_CONFIG` (see below).
- The directory is created `0700` and the file written `0600` — it holds a
  bearer token, same reasoning as `kubectl`.
- A missing file is **not** an error: it resolves to an empty config so `init`
  works on a fresh machine and `version` never needs one.

### How the active context is resolved

In order (first match wins):

1. `$YGGDRASIL_CONTEXT`, if it names an existing context.
2. `current_context`, if set and present.
3. The single context, when exactly one exists.
4. Otherwise "no context" — server-facing commands then fail with
   `no server configured: run 'yggdrasil init' or 'yggdrasil login --server=<url>'`.

You rarely edit this file by hand. `init` creates a context (`local` by default,
or host-derived when `--server` is used); `login` creates/refreshes any named
context and makes it current.

---

## Environment variables (read by the CLI)

| Variable | Used by | Effect |
|---|---|---|
| `YGGDRASIL_CONTEXT` | all context-resolving commands | Force a specific context for one invocation, overriding `current_context`. |
| `YGGDRASIL_CONFIG` | all | Use this exact config file path instead of `~/.yggdrasil/config.yaml`. |
| `YGGDRASIL_URL` | `install` | Default for `--server` (the core's base URL). |
| `YGGDRASIL_WORKFLOW_RUN_TOKEN` | `install` | Default for `--token` (bearer for the install endpoint). |
| `YGGDRASIL_GITHUB_TOKEN` | `install` | GitHub token for fetching `yggdrasil-quickstart.yaml` from private repos and for GHCR auth. Falls back to `GITHUB_TOKEN`. |
| `GITHUB_TOKEN` | `install` | Fallback when `YGGDRASIL_GITHUB_TOKEN` is unset. |
| `OCI_USERNAME` / `OCI_PASSWORD` | `install` (OCI refs) | Explicit registry credentials, override the GHCR token path. Empty password = anonymous pull. |
| `YGGDRASIL_INTEGRATIONS_DEV_DIR` | `integrations` (workspace dev) | Point the catalog manager at a local dev directory of integration repos. |
| `YGGDRASIL_SURFACES_DEV_DIR` | `surfaces` (workspace dev) | Same, for surfaces. |

> `--server` / `--token` flags always override both the active context and the
> env defaults for a single command.

### Override examples

```sh
# Run one command against a different saved context
YGGDRASIL_CONTEXT=prod yggdrasil get workflow

# Use a config file dedicated to CI (no clobbering your laptop's contexts)
YGGDRASIL_CONFIG=/etc/yggdrasil/ci.yaml yggdrasil apply -f workflow.yaml

# Install from a private repo + GHCR
export YGGDRASIL_GITHUB_TOKEN=ghp_xxx
yggdrasil install oci://ghcr.io/acme/integration-internal:v1.0.0
```

---

## Multi-context workflows

Add and switch between environments without hand-editing the file:

```sh
# Local stack (created by init)
yggdrasil init                              # → context "local"

# Add a staging context
yggdrasil login --server https://stg.example.com --username ops-bot --context staging

# Add prod
yggdrasil login --server https://yggdrasil.example.com --username ops-bot --context prod

# Use one per command
YGGDRASIL_CONTEXT=staging yggdrasil get workflow
YGGDRASIL_CONTEXT=prod    yggdrasil status
```

`login` sets the new context as current. To pin a default without typing
`YGGDRASIL_CONTEXT` every time, set `current_context` in the file.

---

## The `init`-generated stack `.env`

In **standalone** mode, `yggdrasil init` writes a `docker-compose.yml` and a
`.env` (chmod `0600`) into `--dir`. The `.env` configures the compose stack, not
the CLI itself. Generated values (random Postgres/RabbitMQ passwords) are filled
in at write time.

| `.env` key | Default | Notes |
|---|---|---|
| `YGGDRASIL_CORE_IMAGE` | from `--core-image` | yggdrasil-core image pin. |
| `CORE_HTTP_PORT` | from `--port` (9080) | Host port mapped to the core. |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `yggdrasil` / random / `yggdrasil` | Database credentials. |
| `AUTH_SESSION_TTL_HOURS` | `720` | Session lifetime. |
| `AUTH_SESSION_COOKIE_NAME` | `yggdrasil_session` | Session cookie name. |
| `YGGDRASIL_BOOTSTRAP_ADMIN_USERNAME` | from `--admin-username` | First-run admin (fires once on an empty DB). |
| `YGGDRASIL_BOOTSTRAP_ADMIN_PASSWORD` | from `--admin-password` (or random) | First-run admin password. |
| `YGGDRASIL_BOOTSTRAP_ADMIN_EMAIL` | from `--admin-email` | Optional. |
| `YGGDRASIL_BOOTSTRAP_ADMIN_DISPLAY_NAME` | from `--admin-display-name` (`Admin`) | Optional. |
| `RABBITMQ_USER` / `RABBITMQ_PASSWORD` / `BROKER_URL` | _(commented out)_ | Opt-in AMQP — see below. |

This file holds the bootstrap admin password and DB credentials — store it with
your other secrets.

### AMQP (RabbitMQ) opt-in

The core boots **HTTP-only** by default; the HTTP transport is always available.
To enable the AMQP `rpc.Transport` backend, edit the generated `.env` to
uncomment the `RABBITMQ_*` + `BROKER_URL` lines, then start with the `amqp`
compose profile:

```sh
cd <init --dir>
docker compose --profile amqp up -d
```

Leaving `BROKER_URL` empty skips RabbitMQ entirely.

> The repo-root `.env.example` (`POSTGRES_*`, `CONSOLE_PORT`, `POSSIBLE_ORGS`,
> …) is for the **contributor monorepo** dev stack driven by `task up`, not for
> `yggdrasil init`. See [DEVELOPMENT.md](DEVELOPMENT.md).

---

See [COMMANDS.md](COMMANDS.md) for the flags that consume these settings, and
[USAGE.md](USAGE.md) for the bootstrap walk-through.
