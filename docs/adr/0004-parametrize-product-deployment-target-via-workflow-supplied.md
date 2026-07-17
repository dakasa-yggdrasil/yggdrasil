# ADR-0004: Parametrize Product deployment target via workflow-supplied target_overrides, keep Products immutable

- **Status:** Accepted
- **Date:** 2026-04-10
- **Deciders:** unknown
- **Scope:** yggdrasil-core (workflow engine + product apply executor)
- **Supersedes:** —
- **Superseded by:** —

## Context

Products in Yggdrasil are compiled, immutable, checksum-versioned artifacts. Each Product component hardcodes a `target.integration_instance_ref` (e.g. pointing at a specific Kubernetes cluster). This means one Product manifest can only be applied to one target, forcing either (a) duplicating the Product per environment (validation/staging/prod/ephemeral), which risks drift and violates DRY, or (b) using an `integration_instance` alias-by-convention (e.g. `kubernetes-default`) per environment, which leaks environment complexity into the product design itself.

DaKasa needs one Product manifest (e.g. `dakasa-platform`) to materialize the same ecosystem across validation, staging, prod, and dynamic ephemeral PR environments without duplicating manifests, while preserving the philosophical requirement that Products remain immutable, auditable artifacts.

## Decision

Keep Products immutable and push environment-target dynamism into the **workflow** layer instead. Extend the core's `ApplyProductInstallationRequest` (and the reconcile/observe/discover product operations) with an optional `target_overrides` map: `{ <integration_instance_ref.name found in the Product> → { integration_instance_ref, namespace? } }`. At apply time, core resolves each component's target by checking the override map, and only `target.integration_instance_ref` and `target.namespace` are replaceable this way — `source`, `renderer`, `reconcile`, `requires`, and `depends_on` remain immutable parts of the artifact. If the map is absent, the Product applies with its original targets (default, non-breaking).

Add a new workflow step kind, `use.kind: "product"`, so a workflow can directly dispatch product operations (`materialize`, `installation.reconcile`, `installation.apply`, `installation.observe`, `installation_state.discover`) with `with.target_overrides`, using the workflow's existing `{{ }}` templating (e.g. `{{ inputs.environment }}`) to parametrize the override at dispatch time.

Overrides are validated before apply: the override key must correspond to an actual `integration_instance_ref.name` used by some Product component (else `override_key_not_found`), and the override's target `integration_instance` must exist and be `active`. Matching is by `integration_instance_ref.name` alone (namespace ignored for matching); if two components share that name but different namespaces, both are overridden together — Products needing independent overrides must use distinct names. Authorization on an overridden apply is evaluated as if the target had originally been the overridden one (overrides cannot be used to bypass RBAC/Policy). Every apply emits `product.installation.applied` with `target_overrides_used` in the payload for audit.

Rejected alternatives: product-level templating/parameters (Helm-values style) — breaks the "Products are compiled immutable artifacts" principle and risks non-reproducible products; a "dakasa-renderer" integration that generates Products dynamically — adds needless indirection, and integrations are for external resources, not for internal manifest massaging; forking Products per environment — violates DRY and risks drift; a separate `target_alias` resource — introduces an implicit stateful "env context" per RPC call; `target_placeholders` declared in the Product manifest — forces boilerplate on Products that don't need overrides.

## Consequences

- Enables multi-environment deploy from a single Product manifest, ephemeral PR environments, DR drills, blue/green deploys, and multi-region rollout, all without manifest duplication.
- The feature is additive/backwards-compatible: existing `ApplyProductInstallationRequest` callers and existing workflows using only `use.kind: "integration"` are unaffected.
- Constrains future Product-related features: any new capability that would let a caller mutate `source`/`renderer`/`requires`/`depends_on` at apply time would violate this decision's immutability boundary and needs its own review.
- Landmine: because override matching is by `integration_instance_ref.name` only, a Product author who reuses the same target name across components that should be overridden independently gets unintended coupled overrides — this is a documented convention gap, not enforced by schema.
- Audit trail depends on the event stream (`product.installation.applied.target_overrides_used`); if event stream emission is disabled/unavailable, overrides still function but lose their audit trail (explicitly allowed as non-blocking in the source spec).

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/docs/superpowers/specs/2026-04-10-workflow-product-target-overrides-design.md
