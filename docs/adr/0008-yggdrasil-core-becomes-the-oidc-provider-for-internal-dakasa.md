# ADR-0008: Yggdrasil-core becomes the OIDC Provider for internal DaKasa SSO, brokering Google Workspace

- **Status:** Accepted
- **Date:** 2026-05-05
- **Deciders:** unknown (brainstormed with user 2026-05-05)
- **Scope:** yggdrasil-core (identity), Tartaro, yggdrasil-console
- **Supersedes:** —
- **Superseded by:** —

## Context

DaKasa needs an "enterprise tier" that requires federated internal authentication. Yggdrasil-core is already the platform's source of truth and already has a generic third-party auth framework (`/auth/providers`, `/auth/third-party/*`) used for console operator login via OAuth/OIDC. What's missing is the piece that makes Yggdrasil the central identity authority for *other* DaKasa surfaces (Tartaro, console, future surfaces): a way to issue verifiable tokens those surfaces can consume, instead of each surface owning its own auth.

Phase 1 scope is deliberately narrow: internal SSO only, for `@dakasa.me` accounts, with Tartaro and the Yggdrasil console as the first two OIDC clients. Multi-tenant/B2B SSO and additional identity providers are explicitly deferred to a separate future design.

## Decision

Yggdrasil-core becomes a full OIDC Provider (discovery, JWKS, `/authorize`, `/token`, `/userinfo`, `/end_session`) using `github.com/zitadel/oidc/v3` as the provider library, implementing its `op.Storage` interface against six new PostgreSQL tables (`oidc_clients`, `oidc_auth_requests`, `oidc_auth_codes`, `oidc_refresh_tokens`, `oidc_signing_keys`, `oidc_provider_settings`, plus `oidc_audit_events`). Google Workspace (`dakasa.me`) is registered as the identity broker — Yggdrasil never re-implements password auth, it delegates real authentication to Google and issues its own tokens on top.

Key sub-decisions (from the design's decisions table):
- **SSO audience**: internal (DaKasa staff) only for Phase 1 — B2B, consumer, and multi-audience SSO are rejected for this phase.
- **MVP clients**: Tartaro + the Yggdrasil console (not just Tartaro alone, not all four surfaces at once).
- **Console packaging**: the console SPA is embedded directly into the yggdrasil-core binary via `embed.FS` (multi-stage Vite+Go Dockerfile) rather than run as a separate BFF — the console becomes an OIDC confidential client that is part of core's own binary.
- **Protocol**: full OIDC (not a custom JWT+JWKS scheme, not cookie+introspection).
- **RBAC mapping is hybrid**: any `@dakasa.me` login auto-provisions into a domain-default Team (`dakasa-internal`); an admin manually promotes to higher-privilege teams (`yggdrasil-admin`, `tartaro-mod`) via a bootstrap CLI or console — Google Workspace group sync is deferred to Phase 2, manual-only and domain-only mapping were both rejected as insufficient.
- **Tartaro cutover is direct, no feature flag**: password-login code is removed outright; rollback path is redeploying the previous Tartaro binary, not toggling a flag.
- **Logout is local only** (Phase 1): revokes the Yggdrasil-issued refresh token and clears the session cookie, but does not perform federated Google logout (SLO deferred to Phase 2).
- **Token policy**: access/ID tokens are stateless JWTs (RS256, 15 min TTL); refresh tokens are opaque, DB-persisted, 7-day sliding, and rotate on every use with replay detection per BCP-225 (a replayed old refresh token revokes its entire rotation chain via a `WITH RECURSIVE` walk over `rotated_from`). PKCE is mandatory. Signing keys rotate quarterly with a 7-day grace overlap; `private_pem` is stored as plaintext in Phase 1 (explicitly deferred to managed-secret storage in Phase 2 — a known, accepted interim risk).
- **Claim scope**: Yggdrasil's issued JWTs carry `email`, `name`, and `teams` — explicitly *not* `roles`/`permissions`. Each consuming surface decides its own authorization locally based on `teams`, keeping the identity provider from becoming an authorization engine for surfaces it doesn't own.
- **Session cookies use `SameSite=Lax`** (not `Strict`) specifically because `Strict` would break the cross-origin redirect back from Google's OAuth flow.

## Consequences

- Yggdrasil-core takes on a new, security-sensitive responsibility (issuing and verifying identity tokens for other DaKasa surfaces) — any future surface that wants to authenticate DaKasa staff is expected to become an OIDC client of Yggdrasil rather than build its own auth.
- Tartaro's password-based login is permanently removed as part of this rollout (direct cutover, no dual-path) — there is no "fall back to password" once this ships; rollback is a full binary redeploy.
- The `teams`-only (no `roles`/`permissions`) claim design constrains every current and future OIDC-consuming surface to implement its own authorization mapping from team membership — Yggdrasil will not centralize per-surface permission logic through this channel.
- Storing `private_pem` in plaintext in Phase 1 is an accepted interim landmine; any audit of secrets-at-rest before Phase 2 ships will find the OIDC signing key unprotected by the managed-secrets encryption path.
- Local-only logout means a user who logs out of Tartaro or the console is still logged into Google — full single-logout is explicitly future work, not a bug to fix under this decision.
- This design is scoped to internal SSO; extending to B2B/multi-tenant customer SSO or additional identity providers (Apple, Microsoft, Facebook) requires a new design, not an extension of this one — those were deliberately pushed to "Phase 3, tracked as follow-up, separate design doc."

## Related
- scratch: /Users/dakasa/projects/dakasa/yggdrasil/yggdrasil/services/yggdrasil-core/docs/superpowers/specs/2026-05-05-yggdrasil-oidc-provider-design.md
