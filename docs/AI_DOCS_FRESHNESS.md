# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: e1a80c4728ce0a03adb04460ec4d1559464d1e04
verified_at: 2026-09-25
by: Claude
note: Reconciled the init stack docs (CONFIGURATION, USAGE, DEVELOPMENT) with the explicit development YGGDRASIL_ENV, the forwarded YGGDRASIL_DEPLOY_TOKEN and the reachable-host caveat in the compose and .env assets, and scoped the posture table to the routes each setting governs on a Core with yggdrasil-core ADR-0022.
