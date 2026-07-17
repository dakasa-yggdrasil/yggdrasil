# ADR-0002: Core enforces BuildProject ephemeral expiration via a background lifecycle loop with an explicit state machine

- **Status:** Accepted
- **Date:** 2026-04-10
- **Deciders:** unknown
- **Scope:** yggdrasil-core (topology / BuildProject)
- **Supersedes:** —
- **Superseded by:** —

## Context

`BuildProject` already carries `Ephemeral bool` and `ExpiresAt string` fields, declaring intent that a build project should self-destruct after a point in time. Nothing enforced that intent — an ephemeral BuildProject past its `ExpiresAt` stayed in the system forever unless a human deleted it manually. This directly contradicts Yggdrasil's core philosophy that "ecosystems should become banal and disposable": if the schema promises expiration and nothing honors it, the promise is decorative and ephemerals leak.

Because lifecycle enforcement of a core-owned entity is inherently a core-state-management concern (an external cleanup integration would have to poll core asking "what expired?" and then drive core's state from outside), this cannot be delegated to an integration or surface.

## Decision

Add a core background loop (new addon `buildproject_lifecycle`) that enforces a formal BuildProject lifecycle state machine: `active → expiring → deleted → hard-deleted`.

- **`active → expiring`**: the loop finds ephemeral BuildProjects with `expires_at < now()` and atomically transitions them via `UPDATE ... WHERE lifecycle_status = 'active' AND expires_at < NOW()` (optimistic locking; a losing concurrent worker sees `affected = 0` and skips). The transition, the `buildproject.expired` event emission, and an optional teardown-workflow dispatch all happen in one transaction — if any part fails, everything rolls back and the BuildProject remains a future candidate.
- **Optional per-BuildProject teardown workflow**: a `TeardownWorkflowRef` field lets each BuildProject declare a workflow to run on expiration (e.g. drop databases, delete a Kubernetes namespace, remove DNS records). If configured, `expiring → deleted` waits for that workflow run to report `succeeded`; on `failed`, the BuildProject stays in `expiring` and a `buildproject.teardown_failed` event is emitted for operator intervention (no automatic retry at MVP). If no teardown workflow is configured, the transition to `deleted` is immediate.
- **`deleted` is a soft delete**: the row stays for a configurable retention (`BUILDPROJECT_HARD_DELETE_RETENTION_DAYS`, default 30) before an separate pass hard-deletes it (0 disables hard-delete entirely). Soft-delete-first is deliberate — it preserves audit trail and allows a `topology.build_project.restore` RPC to reverse a `deleted` (but not yet hard-deleted) BuildProject, e.g. for human error recovery. `restore` explicitly does not re-provision already-torn-down infrastructure.
- **Concurrency-safe by design for multi-worker core**: every state transition uses a conditional SQL `UPDATE ... WHERE <expected prior state>` rather than a distributed lock, so N core workers running the same loop independently converge without double-processing.
- **New management RPCs**: `topology.build_project.extend` (push out `ExpiresAt` while `active`, enabling "extend on activity" patterns), `topology.build_project.expire_now` (force immediate expiration), `topology.build_project.restore` (undo a soft delete).
- Every state transition emits an event on the event stream (`buildproject.created/extended/expired/teardown_started/teardown_completed/teardown_failed/deleted`; `hard_deleted` is optionally skipped since the event history is the durable record, not the row).

Rejected alternatives: delegating cleanup to an external cron/Kubernetes CronJob calling core RPCs (fragments state management outside core, adds moving parts and failure modes); modeling the lifecycle as a long-running Temporal workflow (adds a heavy dependency core is meant to avoid; a background loop with optimistic locking solves it in ~100 lines of Go); immediate hard-delete on expiration with no soft-delete stage (loses audit trail, is irreversible, conflicts with core's mutation-history philosophy); reactive `LISTEN/NOTIFY`-based lifecycle instead of a periodic loop (awkward for time-based expiration which needs a per-row timer, and doesn't dedupe cleanly across multiple workers).

## Consequences

- Depends on the event stream primitive for lifecycle event emission — can be implemented in parallel with it, but is not "complete" (in the audit-trail sense) until the event stream exists.
- Establishes the formal state machine (`active/expiring/deleted/hard-deleted`) as the canonical shape for any future core entity that needs TTL-based lifecycle enforcement — a template to reuse rather than reinvent per-entity ad hoc expiry logic.
- Accidental expiration of an important BuildProject is a named high-impact risk; mitigation is that `ephemeral=true` is an explicit opt-in flag, and RBAC on `build_project.create` is expected to restrict who can set it — this is a process/authorization safeguard, not a technical one.
- A teardown workflow stuck for hours/days leaves the BuildProject parked in `expiring` indefinitely (a configurable timeout only emits a `teardown_timeout` event; it does not auto-resolve) — requires operator intervention, by design, at MVP.
- The loop's own health (does it still run at all?) is a monitoring gap called out as a real risk — mitigation relies on external monitoring (e.g. Heimdall health-check metrics) rather than a built-in core alerting mechanism.

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/docs/superpowers/specs/2026-04-10-buildproject-lifecycle-enforcement-design.md
