# ADR-0006: Standardize all core list RPCs on cursor-based (not offset-based) pagination

- **Status:** Accepted
- **Date:** 2026-04-10
- **Deciders:** unknown
- **Scope:** yggdrasil-core (all list RPCs)
- **Supersedes:** —
- **Superseded by:** —

## Context

Core's list RPCs (`manifest.list`, `collaborator.list`, `team.list`, `product.list`, `workflow.list`, `topology.*.list`, etc. — 13 RPCs total) return every matching result with no limit or cursor. In small ecosystems this is fine; at scale it produces high latency, unbounded core memory use while serializing, risk of hitting RabbitMQ's message-size limit, and forces surfaces to fetch everything and paginate client-side. Because core is the only place that can see and limit the full result set, pagination has to be a core capability — surfaces cannot paginate what core doesn't already paginate.

## Decision

Add a consistent, cursor-based pagination convention to all list RPCs:

- **Request**: an optional `pagination: { cursor, limit }` field. `cursor` is an opaque string (server-generated, never constructed by the caller); omitting it starts from the beginning. `limit` defaults to 100 and is capped at 1000.
- **Response**: `pagination: { next_cursor, has_more, total_estimate? }`. `total_estimate` is optional/approximate (e.g. via `EXPLAIN`) and is not implemented in the MVP cut.
- **Cursor encoding**: base64-encoded JSON of `{ last_id, last_sort_value, sort_key }` — opaque to callers, trivially extensible server-side without a breaking change.
- **Cursor-based, not offset/page-number-based**: chosen specifically because `OFFSET` is O(N+limit) in PostgreSQL and offset/page-number pagination duplicates or skips rows when the underlying list mutates between calls. SQL implementation uses tuple comparison, e.g. `WHERE (created_at, id) < ($1, $2) ORDER BY created_at DESC, id DESC LIMIT $3`, with `id` as a deterministic tiebreaker — this requires a composite index `(sort_field, id)` per supported sort.
- **Each RPC declares a default sort** (e.g. `manifest.list` → `updated_at_desc`) and may declare alternative sorts; only the default sort gets an index at MVP, alternatives are added on demand.
- **Consistency guarantee is per-call-snapshot, not cross-call-strong**: items added/removed/reordered between pagination calls may or may not appear in later pages depending on how they relate to the cursor value — documented as identical in spirit to the event stream's pull semantics, for architectural consistency.
- **Breaking change accepted deliberately, no legacy flag**: callers that previously received "everything" now receive at most 100 items by default. A `legacy_full_list` opt-out flag was considered and explicitly rejected for simplicity — old callers must migrate to cursor-based iteration or pass `limit: 1000`.

Rejected alternatives: offset-based (`LIMIT/OFFSET`) — O(N) scan cost, duplicate/skipped rows under concurrent mutation; page-number pagination — same problems as offset, plus misleading UX ("page 3" is not stable); keyset pagination on a single timestamp without an id tiebreaker — collides/duplicates on identical timestamps; GraphQL-style Relay global cursors — unnecessary complexity for this RPC style.

## Consequences

- All 13 affected list RPCs must implement the identical `PaginationRequest`/`PaginationResponse` shape (`model/pagination.go`, `repository/pagination.go`) — no RPC-specific pagination variants.
- Every list-backing table needs a composite `(sort_field, id)` index for its default sort; adding alternative sorts multiplies index count and write cost, so alternative sorts are added only on demonstrated demand, not preemptively.
- Consumers built against the old "returns everything" behavior break silently (they now get ≤100 items with no error) unless they read `has_more`/`next_cursor` — this must be called out in the CHANGELOG and surfaces (yggdrasil-api, yggdrasil-console, `ygg` CLI) must be updated to pass pagination through before/alongside this rollout.
- `event_stream.pull` is explicitly exempt/unaffected — it was already cursor-paginated by design (see the event stream primitive decision) and this decision aligns the rest of core with that existing pattern rather than the reverse.

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/docs/superpowers/specs/2026-04-10-rpc-pagination-design.md
