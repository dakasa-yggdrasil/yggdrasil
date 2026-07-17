# ADR-0005: Separate Yggdrasil into Core / Integrations / Surfaces, each independently removable

- **Status:** Accepted
- **Date:** 2026-04-10
- **Deciders:** unknown
- **Scope:** yggdrasil (platform-wide architecture)
- **Supersedes:** —
- **Superseded by:** —

## Context

Yggdrasil aims to be a competitive infrastructure/ecosystem orchestration product (compared against Backstage, Crossplane, ArgoCD, Helm, Terraform Cloud, Pulumi, Port). A product audit needed to decide whether to pursue feature-parity with these tools or a different strategy, and needed a durable rule for where any new capability should live so the core does not accumulate scope indefinitely.

The core vision: "ecosystems should become banal and disposable — as easy as deploying a single artifact, but far more granular." To support that, the system needs a stable separation of concerns so that auth, UI, and external-provider logic can all be swapped without touching the engine that manages ecosystem state.

## Decision

Architect Yggdrasil as three kinds of pieces, exactly one of which is mandatory:

- **Core** (`yggdrasil-core`) — the only non-removable piece. It manages ecosystem state with full authority: manifest storage/versioning/validation, RBAC + Policy authorization evaluation, the topology graph, BuildProject (ephemeral) lifecycle, the canonical event stream of state mutations, workflow execution, scheduled/event-driven dispatching, and product materialize/apply/observe orchestration.
- **Integrations** — pluggable connectors to external providers (AWS, GCP, Kubernetes, GitHub, RabbitMQ, Grafana, Heimdall, etc.), invoked via RPC. Integrations own everything that "concerns the external world": provider API calls, webhook receivers, external event consumers, notification dispatch, audit-log shipping, backup automation.
- **Surfaces** — human/API-facing edges (REST API, web console, CLI, auth/JWT). Surfaces are also removable — e.g. a user can replace `yggdrasil-identities` or `yggdrasil-console` with their own. Auth is explicitly a surface concern; core accepts an optional identity forward via an `auth` field on RPC requests but does not implement authentication itself.

Contracts between these pieces (RPC requests/responses, event schemas, manifest schemas) are defined **contract-first as JSON Schema** in `docs/contracts/`, not as shared Go types — so integrations and surfaces can be written in any language without importing `yggdrasil-core`.

The placement test for any new feature: "Can this be an integration? Can this be a surface? Only if neither, does it belong in core." Concretely: external provider operations, webhook receivers, event consumers, notification/audit shippers → integration. REST/GraphQL/CLI/web UI, auth providers, OpenAPI export → surface. Manifest storage, authorization evaluation, topology graph, ephemeral lifecycle, event stream emission, workflow execution and dispatch scheduling → core, because only core has the canonical, transactionally-consistent view of ecosystem state.

Consequently several items that look like "missing features" are deliberately excluded from core: a shared plugin SDK (contract-first > language-first), a plugin marketplace, custom manifest kinds beyond the 7 canonical kinds, custom renderers beyond raw_k8s/kustomize/helm, product-level templating (parametrization is a workflow concern, not a product concern), built-in backup automation, direct Terraform/ArgoCD/Flux integration (Yggdrasil competes with these, it doesn't wrap them by default), a WebSocket/streaming API in core (streaming is a surface concern layered on core's synchronous RPC), and rollback-of-products (delegated to the underlying substrate, e.g. `kubectl rollout undo`).

## Consequences

- Every future feature proposal must be triaged through the core/integration/surface test before implementation; this is the guardrail against core feature-creep explicitly named as the top identified risk.
- Core must stay operable with minimal mandatory dependencies (PostgreSQL, optionally RabbitMQ) — heavier dependencies (Temporal, Kafka, etc.) are pushed to integrations or rejected outright, which shows up as a recurring rationale in later Yggdrasil core specs (event stream, workflow triggers, BuildProject lifecycle) that explicitly reject Temporal/Kafka/RabbitMQ-pubsub alternatives on these grounds.
- Because contracts are JSON Schema rather than shared Go types, any core RPC/event/manifest change must ship a schema update in `docs/contracts/` — this is the language-agnostic extensibility promise and cannot be silently bypassed with Go-only type changes.
- Positioning follows from the split: Yggdrasil is pitched as composability-first (assemble your own platform) rather than feature-complete-first (Backstage/Port), and as push-based-by-default but pull-capable-by-design (vs. ArgoCD's pull-only GitOps), and as an orchestrator that can *absorb* Helm as a renderer rather than compete with it.
- Landmine: because auth and console are removable surfaces, core intentionally does not enforce authorization on its own RPCs beyond what an optional `auth` field enables — trust-boundary enforcement is a surface's job. A surface that forgets this exposes an effectively unauthenticated core.

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/docs/superpowers/specs/2026-04-10-yggdrasil-product-audit-report.md
