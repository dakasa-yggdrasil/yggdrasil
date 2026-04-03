# Repo Context

Use this context whenever working in the Yggdrasil product monorepo.

## Identity
This repo defines the product runtime and orchestration model.

## Default workflow
1. Identify whether the change belongs in core, a surface, or a standalone integration repo.
2. Prefer core-owned domain logic over surface-owned logic.
3. Run `task arch:check` and `task config`.
4. Run `task smoke` for runtime changes.
5. Run `task build:images` for packaging changes.
