# ADR-0001: Add a durable, pull-based, PostgreSQL-backed event stream as a foundational core primitive

- **Status:** Accepted
- **Date:** 2026-04-10
- **Deciders:** unknown
- **Scope:** yggdrasil-core
- **Supersedes:** —
- **Superseded by:** —

## Context

Core manages ecosystem state (manifests, topology, products, workflows) but has no unified, canonical trail of state mutations. Several needed capabilities — audit log/compliance export, workflow run history, BuildProject lifecycle history, a real-time activity feed, Slack/email notifications on critical events, event-driven workflow triggers, and cost tracking — all reduce to "consume a stream of state-change events." Without a shared primitive, each of these would reimplement its own ad-hoc events/polling infrastructure with separate tables, fragmenting state and violating the "core manages state with mastery" principle.

Only core has the canonical, transactionally-consistent view of its own mutations; an external integration cannot "intercept" core mutations without incorrect coupling, and a surface only consumes/exposes, it doesn't originate events. So the primitive has to live in core.

## Decision

Add an `event_log` PostgreSQL table and a pull-based RPC (`yggdrasil-core.event_stream.pull`) as the durable, canonical event stream:

- **Transactional emission**: `events.Emit(tx, req)` must be called with the caller's existing `*sql.Tx`, inside the same transaction as the state mutation it represents. If the transaction rolls back, the event does not exist. This gives the "if you see the mutation, the event exists" guarantee.
- **Persistence and ordering**: each event has a UUID v7 `event_id` (time-ordered) and a PostgreSQL `BIGSERIAL sequence` (global monotonic counter shared across all core workers). Per-aggregate ordering is guaranteed via `aggregate_type`/`aggregate_id`; cross-aggregate ordering is monotonic-by-sequence but not strictly causal — accepted as a trade-off.
- **Pull-based, not push**: subscribers call `event_stream.pull` with an opaque cursor + filters (`types` with wildcard support, `aggregate_type`, `aggregate_id`, `supported_schema_versions`, `emitted_after`) and a limit; response includes `next_cursor` and `has_more`. No push callbacks, no core-side slow-consumer backpressure — consumers control their own pace.
- **Contract-first, schema-versioned payloads**: every event type has a JSON Schema in `docs/contracts/events/v1/{category}/{type}.schema.json`, validated at emit time. `v1` is forever non-breaking (only additive fields); breaking changes create a new `v2` type that coexists.
- **Configurable retention**: a PostgreSQL `event_retention_policy` table (pattern → TTL days, most-specific-match-wins) drives a background cleanup job running hourly; defaults are 90 days catch-all, 7 years for `authorization.*` (compliance), forever for `manifest.*`, 365 days for `buildproject.*`, 30/180 days for workflow step/run events.
- **No backfill**: the stream starts empty at deploy time; pre-existing manifests/products are not retroactively converted into events, to avoid fabricating events with incorrect actors/timestamps.
- **Sensitive-data convention**: events never carry secret values in clear, only `secret_ref` pointers; schema validation is expected to reject fields like `credentials`/`password`/`api_key` in payloads.
- **No core-side authorization on pull (MVP)**: core trusts any RPC caller (surfaces are the trust boundary); a surface wanting per-collaborator event access control must enforce it itself, or pass an `auth.collaborator_id` for core to optionally evaluate RBAC/Policy on `events/{type_pattern}`.

Rejected alternatives: PostgreSQL `LISTEN/NOTIFY` (8KB payload limit, non-durable, requires a dedicated connection per subscriber); RabbitMQ fanout exchange (couples the transport-agnostic core to RabbitMQ specifically, non-durable by default); external Kafka/NATS (heavy dependency, hard for arbitrary-language consumers); pure event sourcing where `event_log` replaces entity tables (massive refactor, slower current-state queries) — event stream is additive to state tables, not a replacement.

## Consequences

- This is explicitly the foundational primitive several other core features depend on: BuildProject lifecycle enforcement, the workflow trigger system (schedule + event), and future audit-log/activity-feed/cost-tracking projections all build on `event_stream.pull`.
- Write amplification is accepted (every state mutation now writes to its own table AND `event_log`) in exchange for auditability; if this becomes a real bottleneck, PostgreSQL declarative partitioning by month is the documented escape hatch (not implemented at MVP).
- Because retention is destructive (hard delete, no soft-delete/undo), reducing a retention TTL makes previously-safe events eligible for deletion on the next cleanup pass — operators must be careful when lowering TTLs.
- Because cursors advance per raw sequence rather than per matched-filter event, subscribers with narrow filters can see many empty pulls when most events don't match — documented as an accepted MVP simplification with a known future optimization (advance cursor to last matching event).
- Any future consumer of core mutations (SIEM export, Slack bot, cost tracker, etc.) is expected to be built as a pull subscriber against this stream rather than inventing a new notification mechanism.

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/docs/superpowers/specs/2026-04-10-event-stream-design.md
