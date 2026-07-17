# ADR-0007: Yggdrasil-core itself reconciles managed secrets into Kubernetes, via a generic hybrid reactive+periodic Materializer interface

- **Status:** Accepted
- **Date:** 2026-04-12
- **Deciders:** unknown
- **Scope:** yggdrasil-core (managed secrets → Kubernetes bridge, yggdrasil-yggdrasil repo)
- **Supersedes:** —
- **Superseded by:** —

## Context

Yggdrasil-core already owns "managed secrets" as first-class API resources: full CRUD, versioning, rotation, and namespacing, stored in PostgreSQL, acting as the DaKasa platform's single control plane. Consumers of these secrets are Kubernetes workloads — both in-cluster and, for some targets, remote clusters reached via a stored kubeconfig — that expect ordinary native `Secret` objects mounted/env-injected the standard k8s way. Nothing bridged "secret record changed in yggdrasil-core's DB" to "K8s Secret object reflects that change." The bridge needs to handle both local (in-cluster ServiceAccount) and remote clusters, avoid redundant writes, and support secret lifecycle states (active, disabled, revoked) without destroying the audit trail on the K8s side.

The External Secrets Operator was considered and explicitly rejected: the position taken is that Yggdrasil itself should be "the operator that connects the realms," not a passive data source for a third-party operator.

## Decision

Build a reconciliation engine (`reconciler/` package) inside yggdrasil-core around a generic `Materializer` interface (`Materialize`, `Reconcile`, `Owns`), with `SecretMaterializer` as its first (and for now, only) concrete adapter — deliberately designed so future resource kinds (Products, RBAC, Policies, ConfigMaps) can reuse the same interface + addon-priority pattern later, rather than requiring a rewrite.

Key sub-decisions:

- **Trigger model is hybrid**: reactive materialization fires near-immediately when HTTP write handlers call `Engine.NotifyChange`, which pushes onto a buffered event channel, *plus* a periodic full reconcile loop (default 60s, overridable via `RECONCILE_INTERVAL`, minimum 5s) that re-lists all active managed secrets and repairs drift. Reactive-only would miss cases where the initial push failed or the K8s Secret was mutated/deleted out of band.
- **Cluster access via `KubeClientPool`**: target `"local"` uses the in-cluster ServiceAccount (`rest.InClusterConfig()`) with zero config; any other target name resolves a kubeconfig by looking up a managed secret at the fixed convention `namespace="global", name="kubeconfig-{target}", key="kubeconfig"`, cached client-side with a 5-minute TTL. Remote-cluster credentials are themselves modeled as yggdrasil managed secrets — no separate credential store. Multi-cluster remote targets are explicitly out of scope for this cut (the interface supports it; no adapter ships yet).
- **Naming convention**: Yggdrasil namespace/name maps 1:1 to Kubernetes namespace/name by default (zero config for the common case), with an explicit override escape hatch via a `metadata.materialize.{namespace,name}` field on the managed secret record — e.g. a `global/stripe-keys` secret can materialize into `dakasa/enterprise-payments-api-secrets`. Target cluster is likewise selectable via `metadata.materialize.target` (defaults to `"local"`).
- **Idempotency/drift-detection via an annotation contract**: every materialized K8s Secret is labeled `yggdrasil.io/managed-by: yggdrasil-core` (the ownership marker — the reconciler only ever touches Secrets carrying this label) and annotated with `yggdrasil.io/secret-version` (compared against the source record's version to decide create/update/skip), plus `source-namespace`, `source-name`, `last-synced`, and — for revoked/disabled secrets — `status` + `revoked-at`.
- **Delete lifecycle is non-destructive**: revoking or disabling a managed secret patches the K8s Secret's annotations (`status: revoked|disabled`, `revoked-at`) in place rather than deleting it — deleting a live Secret could kill running pods; actual removal is a separate, explicit operator decision.
- **Addon registration is non-critical**: the reconciler registers as an optional addon (priority 40). If Postgres is unavailable, `RECONCILE_ENABLED=false`, or in-cluster/kubeconfig setup fails, the addon no-ops rather than failing yggdrasil-core startup — materialization is an enhancement layer over the core secrets API, not a hard dependency.
- **RBAC is narrowly cluster-scoped**: a dedicated `ClusterRole`/`ClusterRoleBinding` (`yggdrasil-reconciler`) grants only `get/list/create/update/patch` on `secrets`, bound to the `yggdrasil` ServiceAccount — a `ClusterRole` (not a namespaced `Role`) because materialization crosses namespaces (e.g. `dakasa`, `infra`).
- Adds `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` as new yggdrasil-core dependencies.

Explicitly out of scope for this decision: multi-cluster remote adapters beyond the interface shape, Product/RBAC/Policy materializers (future, same interface), AWS Secrets Manager sync (Yggdrasil is declared the source of truth, not AWS SM), and encryption-at-rest for managed secrets in PostgreSQL.

## Consequences

- Establishes the `Materializer` interface + addon-priority pattern as the template any future "push Yggdrasil-managed state into an external system" feature (Products, RBAC, Policies, ConfigMaps materialized as K8s objects) should implement, rather than each growing its own bespoke sync mechanism.
- Rejecting the External Secrets Operator means Yggdrasil now owns this reconciliation responsibility long-term — any future adoption of ESO or a similar third-party operator for secrets would be a reversal of this decision, not an addition alongside it.
- The label-based ownership boundary (`yggdrasil.io/managed-by`) is the safety mechanism preventing the reconciler from clobbering unmanaged cluster Secrets; any change to the reconcile-loop query logic must preserve this filter. Version comparison trusts the K8s object's own annotations as the source of truth for "what yggdrasil version is currently reflected" — an out-of-band edit to `yggdrasil.io/secret-version`, or an actor stripping the `managed-by` label, desyncs the object from the reconciler's view without an error surfaced.
- Reactive push is best-effort: the event channel is bounded (64) and drops-with-a-warning-log on overflow, relying on the periodic full reconcile (up to `RECONCILE_INTERVAL` later) to eventually correct any dropped event — consumers should not assume sub-second consistency after a secret write.
- Because delete is non-destructive (annotation-only), stale/dead Secrets can accumulate in the cluster after a managed secret is revoked — cleanup is an explicit, separate operator decision, not automatic.
- Remote-cluster access is coupled to the managed-secrets subsystem itself (kubeconfig-as-managed-secret): losing or rotating a `global/kubeconfig-{target}` secret breaks reconciliation to that cluster, and there is no independent credential path. That same coupling means the remote-cluster attack surface inherits whatever protections exist for the managed-secrets store — encryption-at-rest for that store is called out as a future concern still unaddressed at this decision's time.
- New Kubernetes client dependencies and a new cluster-scoped RBAC surface (`ClusterRole` with secrets create/update) must be provisioned wherever yggdrasil-core runs.

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/services/yggdrasil-core/docs/superpowers/specs/2026-04-12-reconciler-secret-materializer-design.md
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/services/yggdrasil-core/docs/superpowers/plans/2026-04-12-reconciler-secret-materializer.md
