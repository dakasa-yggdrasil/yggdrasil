# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: e26d00796c0173e91e9617167dc57da5a010fb43
verified_at: 2026-09-09
by: Codex
note: Reconciled CLI human-session CSRF writes, request-local credentials, redirect refusal, failure behavior, and existing MFA login options against the client and authenticated Core API contract.
