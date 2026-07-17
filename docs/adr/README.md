# Architecture Decision Records — yggdrasil-yggdrasil

Curated, immutable record of durable architectural decisions — one decision per file.
Working scratch (brainstorming, plans, handoffs) lives in `docs/superpowers/` and is **not
tracked** (see `AGENTS.md` § Spec-driven docs). The domain-wide model is defined in the
monorepo root `docs/adr/0001-adopt-adr-plus-scratch-model.md`. To change a decision, write a
NEW ADR that supersedes the old one; never edit an Accepted ADR's Decision.

**8 decisions.**

| ADR | Title | Status | Date | Scope |
|-----|-------|--------|------|-------|
| [0001](0001-add-a-durable-pull-based-postgresql-backed-event-stream-as-a.md) | Add a durable, pull-based, PostgreSQL-backed event stream as a foundational core primitive | Accepted | 2026-04-10 | yggdrasil-core |
| [0002](0002-core-enforces-buildproject-ephemeral-expiration-via-a-backgr.md) | Core enforces BuildProject ephemeral expiration via a background lifecycle loop with an explicit state machine | Accepted | 2026-04-10 | yggdrasil-core (topology / BuildProject) |
| [0003](0003-core-owns-workflow-scheduling-native-cron-scheduler-event-st.md) | Core owns workflow scheduling — native cron scheduler + event-stream-subscribed dispatcher, not delegated to an external integration | Accepted | 2026-04-10 | yggdrasil-core (workflow engine) |
| [0004](0004-parametrize-product-deployment-target-via-workflow-supplied.md) | Parametrize Product deployment target via workflow-supplied target_overrides, keep Products immutable | Accepted | 2026-04-10 | yggdrasil-core (workflow engine + product apply executor) |
| [0005](0005-separate-yggdrasil-into-core-integrations-surfaces-each-inde.md) | Separate Yggdrasil into Core / Integrations / Surfaces, each independently removable | Accepted | 2026-04-10 | yggdrasil (platform-wide architecture) |
| [0006](0006-standardize-all-core-list-rpcs-on-cursor-based-not-offset-ba.md) | Standardize all core list RPCs on cursor-based (not offset-based) pagination | Accepted | 2026-04-10 | yggdrasil-core (all list RPCs) |
| [0007](0007-yggdrasil-core-itself-reconciles-managed-secrets-into-kubern.md) | Yggdrasil-core itself reconciles managed secrets into Kubernetes, via a generic hybrid reactive+periodic Materializer interface | Accepted | 2026-04-12 | yggdrasil-core (managed secrets → Kubernetes bridge, yggdrasil-yggdrasil repo) |
| [0008](0008-yggdrasil-core-becomes-the-oidc-provider-for-internal-dakasa.md) | Yggdrasil-core becomes the OIDC Provider for internal DaKasa SSO, brokering Google Workspace | Accepted | 2026-05-05 | yggdrasil-core (identity), Tartaro, yggdrasil-console |
