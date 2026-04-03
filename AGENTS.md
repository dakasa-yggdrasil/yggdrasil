# AGENTS

## Repo role
This repository is the Yggdrasil product monorepo. `yggdrasil-core` is the heart of the product. Surfaces are replaceable edges. Integrations are installable external runtimes.

## Architectural rules
- Keep business authority inside `services/yggdrasil-core`.
- Surfaces must stay thin. They should not become alternative sources of truth.
- Avoid direct imports across services and surfaces. Use published contracts and runtime boundaries.
- If a change affects runtime contracts, update docs, tests, bootstrap data, and the console flow in the same pass.
- Integrations are optional runtime components, not domain owners.

## Mandatory validation
- `task arch:check`
- `task config`
- `task smoke` for runtime or API changes
- `task build:images` for release/runtime packaging changes

## Working style
- Prefer small, explicit manifests and typed contracts over hidden conventions.
- Keep docs and bootstrap manifests aligned with code.
- Preserve submodule boundaries for reference surfaces.
