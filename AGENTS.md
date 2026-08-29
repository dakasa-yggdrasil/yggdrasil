# AGENTS

## Repo role
This repository is the Yggdrasil product monorepo. `yggdrasil-core` is the heart of the product. Surfaces are replaceable edges. Integrations are installable external runtimes.

## Architectural rules
- Keep business authority inside `services/yggdrasil-core`.
- Surfaces must stay thin. They should not become alternative sources of truth.
- Avoid direct imports across services and surfaces. Use published contracts and runtime boundaries.
- If a change affects runtime contracts, update docs, tests, bootstrap data, and the console flow in the same pass.
- Integrations are optional runtime components, not domain owners.

## Feature-linked deep audits

For a deep audit, exhaustive sweep, or production-readiness review, work on one complete feature root at a time. Expand it as a linked tree from every entry point through components, state, contracts, transports, persistence, workflows, integrations, external effects, observability, retries, and cleanup. Close the feature with evidence and validation, store a compact evidence capsule, release its detailed context, and only then select the next feature. Audit shared nodes once and reopen every dependent feature if a shared-node change invalidates earlier evidence.

Repository authority is a hard boundary. Discovery may identify an external edge, but a repository-scoped collaborator must stop there: do not inspect the external implementation deeply, edit it, or claim it works. Write a concrete Definition of Done for the owning repository/team with producer evidence, expected consumer contract, reproduction, acceptance criteria, compatibility constraints, required tests, and closure evidence. Cross-repository traversal is allowed only when the user explicitly grants aggregator/workspace scope. An explicitly authorized `dakasa-system` audit may traverse the full DaKasa and Yggdrasil universe, while still obeying each repository's nearest policies and ownership contracts.

The full procedure and completion contract are in `docs/feature-linked-deep-audit.md`. Never call a broad audit complete from a repository list, route inventory, mocked unit suite, or truncated output alone; completion requires feature inventory coverage, edge-by-edge evidence, and explicit blocked or skipped nodes.

## Mandatory validation
- `task arch:check`
- `task config`
- `task smoke` for runtime or API changes
- `task build:images` for release/runtime packaging changes

## Working style
- Prefer small, explicit manifests and typed contracts over hidden conventions.
- Keep docs and bootstrap manifests aligned with code.
- Preserve submodule boundaries for reference surfaces.


## Spec-driven docs (ADR + scratch) — mandatory

Two layers, federated per repo (the domain-wide model is defined in the monorepo root
`docs/adr/0001-adopt-adr-plus-scratch-model.md`):

- **`docs/adr/NNNN-title.md` — the tracked, curated record.** One durable architectural
  decision per file. Write an ADR ONLY when a decision has lasting architectural consequence
  (a contract, a topology choice, a convention others must follow). Not for routine work.
  Template: `docs/adr/TEMPLATE.md`. Index: `docs/adr/README.md`.
- **`docs/superpowers/**` — gitignored working scratch.** Brainstorming specs, implementation
  plans, and sub-session handoffs live here. Local-only; git does not track it. Disposable.

**Immutability:** never rewrite the Decision of an Accepted ADR. To change a decision, write a
NEW ADR that supersedes the old one and flip the old one's Status to `Superseded by NNNN`.

**Handoffs:** a sub-session still writes its handoff to `docs/superpowers/` and the main agent
still reads it off disk (anti-race). It is scratch — NOT versioned. If the session made a
durable decision, record that as an ADR; the handoff itself is disposable.

**Staleness:** a recalled memory or a `docs/superpowers/` scratch spec may be outdated — verify
against the current-status ADR and the code before asserting it as fact.

**Context files are durable-only (freshness discipline).** `CLAUDE.md` and `AGENTS.md` hold only current-state, durable instructions and policies — things expected to stay true. They MUST NOT accumulate time-bound content: no "recent work" or session-log sections, no dated phase/deploy status, no commit SHAs cited as progress, no machine-specific absolute paths. That content rots inside a durable file. Route a lasting decision to an ADR (`docs/adr/`, immutable, point-in-time); route transient status to gitignored `docs/superpowers/` scratch. Enforced in CI by `.github/workflows/context-freshness.yml` (fails on reintroduced time-bound sections and on structurally-broken ADRs).

## Docs freshness (AI-reconciled, stamp-gated) — mandatory

**Before you open a PR or merge, update every doc your change makes stale.** If you touched an
integration contract, an op signature, a capability, a surface, an event, or any behavior a doc
describes, the matching doc under `docs/` (ADRs, contracts, the map) and any affected README or
doc-comment must move in the same change. A doc that still describes the old behavior is a false
witness the next reader trusts.

**Prove it with the stamp.** Each docs tree carries `docs/AI_DOCS_FRESHNESS.md` with
`verified_at_commit` (the commit an AI or agent-assisted human last reconciled those docs at).
When you reconcile the docs, bump that field to your branch tip. On arrival, if the stamp is
behind the code you are about to touch, reconcile those docs FIRST, before trusting them.

**CI is the safety net.** `.github/workflows/docs-freshness.yml` runs on every PR: a cheap gate
checks whether real code changed and the stamp was NOT bumped; only then does an AI reconcile
the stale docs, commit ONLY the doc files back to the PR branch, and bump the stamp. If your
agent already reconciled and stamped, the gate skips the AI (the economy path). The reconciler
never weakens a doc to match a bug and never touches source.

**Prose docs carry a human-feel layer.** Machine-precise docs (service contracts, schemas, the
freshness stamp) come first, but whenever you write or update a PROSE doc (a README, an ADR, an
architecture narrative, a runbook, a connectivity view), make it legible at a glance: a mermaid
diagram for anything with a flow, a graph, or a sequence; real markdown tables, never ASCII art;
short sections over walls of text. IA first, human feel beside it.
