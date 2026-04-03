This is the Yggdrasil product monorepo.

Rules:
- Keep business authority inside `services/yggdrasil-core`.
- Keep surfaces thin and replaceable.
- Use `task arch:check`, `task config`, and `task smoke` for meaningful runtime changes.
- Update manifests, docs, and tests together when contracts move.
