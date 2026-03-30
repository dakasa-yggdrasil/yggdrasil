# __SURFACE_DISPLAY_NAME__

This repository is a reference starting point for a Yggdrasil surface.

A surface is an edge runtime. It is intentionally not part of the product heart.
The expected shape is:

- talk to `yggdrasil-core` over its synchronous API
- consume core contracts instead of owning platform truth
- keep integrations, workflows, products, and authorization inside the core

## What To Rename First

When you create a new surface from this template, update at least:

- `go.mod`
- `docker-compose.yml`
- `Taskfile.yml`
- `surface.manifest.json`
- the service name and ports in `.env.example`

## Local Development

The default `.env.example` assumes this repo is installed inside the Yggdrasil
product workspace at `surfaces/<your-surface>`.

If you are working on this repo outside the monorepo, set
`YGGDRASIL_WORKSPACE_ROOT` to the absolute path of your local `yggdrasil`
workspace before running the tasks. The `Taskfile` resolves the local source
directory automatically, so the same repo can run as a standalone checkout or as
an installed submodule inside the product workspace.

```bash
task up
task logs
task test
```

The example manifest for registering this surface in the core lives in
[`surface.manifest.json`](./surface.manifest.json).
