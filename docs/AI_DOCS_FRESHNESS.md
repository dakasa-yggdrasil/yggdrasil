# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 71b406793094f362e9eaa9175a6cf3643d4120f5
verified_at: 2026-10-26
by: Codex
note: Reviewed hidden terminal input, exact active workflow schema/version binding, one async dispatch through existing human-session CSRF, fixed private errors and UUID/status-only output. Existing ordinary input behavior and server authority are preserved. Commands and README match the implementation; CI builds an immutable macOS CLI artifact after unit checks. Generated context was refreshed. No secret, live dispatch or local executable validation occurred.

verified_diff_sha256: 796add7e573ceacb82d481f742c59bcb349b4254b74f859b8b0b5ee4ccc5a29f

reconciler_schema: 1
