# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: c1d336d3c30da89b46598f4d3926104183794311
verified_at: 2026-10-29
by: Giomaster
note: Reconciled the removal of the body-asserted third-party login step from scripts/smoke.sh (yggdrasil-core ADR-0088 removed POST /api/v1/auth/third-party/login). The smoke keeps the browser OIDC flow through the mock provider as its third-party login check. No doc describes the removed step: docs/DEVELOPMENT.md, docs/local-development.md, docs/production-readiness.md and CLAUDE.md name task smoke only, and docs/COMMANDS.md already says provider apply prints the start URL. Static checks only (bash -n); the live smoke runs in the dispatch-only product CI job.
