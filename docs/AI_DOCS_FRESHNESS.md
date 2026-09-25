# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: c35e14f12c1c46c04fb93031ceef0e5c24bd4b7b
verified_at: 2026-09-25
by: Claude
note: Reconciled the init stack docs (CONFIGURATION, USAGE, DEVELOPMENT) with the explicit development YGGDRASIL_ENV, the forwarded YGGDRASIL_DEPLOY_TOKEN and the reachable-host caveat in the compose and .env assets, and scoped the posture table to the routes each setting governs on a Core with yggdrasil-core ADR-0022. Deploy-family rows follow yggdrasil-core#57: refused unless YGGDRASIL_ENV is a development value; manifest writes are never credential-free.
