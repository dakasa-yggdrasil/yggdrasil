# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: dcc211de26e65ad2fa80115a5c8172f7b8c871e1
verified_at: 2026-09-24
by: Claude
note: Reconciled the init stack docs (CONFIGURATION, USAGE, COMMANDS, DEVELOPMENT) with the explicit development YGGDRASIL_ENV in the compose and .env assets and the Core posture it keeps (yggdrasil-core ADR-0022).
