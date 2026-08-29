# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 27b58f8da8b6843e01cb886f2a16d6fc912aa7e2
verified_at: 2026-08-29
by: Codex
note: Reconciled the feature-linked audit contract and the standalone CLI/Compose dispatcher paths.
