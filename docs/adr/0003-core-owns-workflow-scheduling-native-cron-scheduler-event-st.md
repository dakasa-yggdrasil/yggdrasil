# ADR-0003: Core owns workflow scheduling — native cron scheduler + event-stream-subscribed dispatcher, not delegated to an external integration

- **Status:** Accepted
- **Date:** 2026-04-10
- **Deciders:** unknown
- **Scope:** yggdrasil-core (workflow engine)
- **Supersedes:** —
- **Superseded by:** —

## Context

The workflow manifest schema already declares `supportedWorkflowTriggerModes = ["manual", "event", "schedule"]`, but only `manual` is implemented — `schedule` and `event` are schema-accepted but silently do nothing, which is worse than not declaring them at all (it creates false expectations). The team initially considered making schedule/event triggers external integrations (e.g. `integration-scheduler-cron`, `integration-webhook-receiver`), then re-calibrated: "core manages state with mastery" was judged to include managing *when* things happen in the ecosystem, not just what happens. Delegating the primary scheduling decision to something outside core would fragment lifecycle management.

## Decision

Implement both trigger modes natively in core, as two related but separably-shippable background subsystems, both declared directly in the workflow manifest (no separate "schedule" manifest kind):

- **Schedule triggers** (`trigger.mode: "schedule"`): a core background loop (new addon `workflow_scheduler`, default 10s tick) evaluates POSIX cron expressions (via `github.com/robfig/cron/v3`) per scheduled workflow, respecting `start_at`/`end_at` and IANA `timezone`. Fire-state (`last_fired_at`, `next_fire_at`) is persisted per-workflow in `workflow_schedule_state`, and dispatch is made idempotent across multiple core workers via an optimistic-locking `UPDATE ... WHERE last_fired_at < $2` (loser sees `affected = 0` and skips). Missed ticks after downtime default to `catchup_policy: "skip"` (not catch-up-fire-all) to avoid dispatch avalanches; `catch_up` is opt-in per workflow. A workflow's schedule can be paused via `trigger.enabled: false`.
- **Event triggers** (`trigger.mode: "event"`): a core background loop (new addon `workflow_event_triggers`) is itself an internal pull-based subscriber of the event stream (see the event-stream-primitive decision), matching each pulled event against registered triggers by `types` (wildcard), optional `aggregate_filter`, and optional `payload_filters` (dotted-path + operator). Only one worker processes each pass (PostgreSQL advisory-lock leader election, `pg_try_advisory_lock`), and the loop persists its own cursor in `workflow_event_trigger_state` for crash recovery. `debounce_seconds` coalesces rapid repeated matching events into a single dispatch (best-effort, in-memory per-worker — lost on crash, accepted). The triggering event is injected into workflow templating as `metadata.triggering_event`.
- **Dispatch semantics**: at-least-once, not exactly-once — if core crashes mid-tick a trigger may fire twice; workflows are expected to be idempotent or use `correlation_id` to dedupe. Every dispatch (regardless of trigger mode) emits `workflow.run.dispatched` with `metadata.triggered_by: "manual" | "schedule" | "event"` for auditability.
- **Integrations may still produce external events** (webhook receivers, Kafka consumers) that land in the event stream, and an external cron integration may coexist for operators who want to reuse existing Kubernetes CronJobs — but the *decision of when to dispatch* stays in core.

Rejected alternatives: scheduling via Temporal (heavy dependency; core is meant to run on just PostgreSQL + optional RabbitMQ; a ~100-line native cron loop is sufficient); an external `integration-scheduler-cron` as the primary mechanism (fragments lifecycle state management out of core — re-calibrated away from this after initial consideration); event triggers via RabbitMQ pub/sub push instead of pulling the event stream (inconsistent with the event stream's pull-based design); a separate `schedule` manifest kind pointing at workflows (adds an artificial new concept when `trigger` already exists on the workflow manifest).

## Consequences

- Hard dependency on the event stream primitive: event triggers cannot exist before/without it; schedule triggers also emit through it for audit visibility.
- Workflows previously created with `trigger.mode: "schedule"` or `"event"` (accepted by the schema but inert) will start firing automatically the moment this ships — a documented potential breaking-behavior change; operators are expected to audit such workflows before upgrading.
- At-least-once dispatch semantics push an idempotency burden onto workflow authors; this is an accepted, explicit trade-off rather than paying for exactly-once delivery.
- Debounce state is best-effort and in-memory — a worker crash during a debounce window silently drops the pending consolidated dispatch; not corrected by design (MVP-acceptable).
- No safeguard yet against a workflow's event trigger causing infinite self-triggering recursion (e.g. reacting to its own `workflow.run.completed`) or unbounded dispatch rate from an overly broad filter — flagged as an open follow-up, not solved at this decision's scope.
- This decision and the BuildProject lifecycle enforcement decision are complementary alternatives for periodic cleanup (a native lifecycle-enforcer loop vs. a scheduled cleanup workflow) — both are valid and may coexist.

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/docs/superpowers/specs/2026-04-10-workflow-trigger-system-design.md
