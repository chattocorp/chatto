# FDR-023: Authentication & Sessions

**Status:** Active
**Last reviewed:** 2026-06-07

## Overview

Chatto authenticates users via two parallel mechanisms: HTTP-only cookie sessions for the embedded SPA (same-origin) and opaque bearer tokens for cross-origin clients (multi-instance frontends, CLI tools, future mobile apps). Login flows include classic password login, OAuth providers, and a bootstrap path for first-boot operator setup.

## Behavior

- **Login** — users sign in with login + password on a `/login` page. The page is also used for redirect-after-signup.
- **OAuth login** — operators can configure OAuth providers (e.g., Google). The login page shows provider buttons; clicking takes the user through the standard authorization-code flow.
- **Cookie session** — on successful auth from the embedded SPA, the server issues an HTTP-only, SameSite=Lax cookie with a 90-day expiry. The cookie carries an opaque session ID plus the user ID needed to derive the lookup key; the authoritative `CookieSession` protobuf record lives in `RUNTIME_STATE` under `cookie_session.{userId}.{hmac}` with a per-key TTL. The server validates the KV record and resolves the current user from projections per request.
- **Bearer token** — every authentication endpoint also issues an opaque token (format: `cht_AT` + 14-char NanoID). Cross-origin clients store it (usually in `localStorage`) and send it as `Authorization: Bearer …` on HTTP requests and `connectionParams.token` on graphql-ws upgrades. The token record lives in `RUNTIME_STATE` as an HMAC-derived `session.{hmac}` key with a per-key TTL.
- **WebSocket auth** — for the embedded SPA, the cookie is automatically attached to the WebSocket upgrade and the user is authenticated before the WS handshake completes. For cross-origin clients, the token in `connectionParams` is checked at upgrade time.
- **Logout** — for cookie sessions: the server deletes the current cookie-session KV record, clears the cookie, and the SPA does a hard reload. For tokens: the client removes the token from `localStorage`; optionally the server revokes the token by deleting its KV key.
- **Session refresh** — cookie-session KV TTL cannot be touched in place, so active sessions are rotated near expiry: the server creates a replacement `CookieSession` record with a fresh TTL, updates the browser cookie, and deletes the old record best-effort. Bearer tokens follow a sliding-window TTL — each successful validation rewrites the `RUNTIME_STATE` entry with a fresh per-key TTL.
- **Password and account lifecycle revocation** — password resets, password changes, and account deletion advance the user's auth generation through durable `EVT` user events. Cookie sessions, bearer tokens, and OAuth authorization codes store the auth generation they were issued against; validation waits the user projection to the current auth-generation events and rejects credentials from older generations. Generation `0` is reserved for pre-field legacy runtime credentials and is upgraded on validation when the credential is not older than the current password event. Revoke-all scans still delete matching `cookie_session.*` and `session.*` records as cleanup.
- **Password reset tokens** — reset links are backed by `RUNTIME_STATE` HMAC-derived `password_reset.{hmac}` records with a 1-hour per-key TTL. Raw reset tokens and links are never written to `EVT` or backup archives.
- **Server version handshake** — the WebSocket `connection_ack` payload includes the server's version. The frontend uses this to detect deployed-version drift and prompt the user to refresh.
- **Auth audit facts** — successful cookie logins, failed password login attempts, logout completion, bearer-token issuance/revocation, OAuth authorization-code issuance/exchange, registration-link issuance, password-reset link issuance, and password-reset completion are appended to `EVT` for admin audit-log inspection. Payloads carry safe request metadata only: capped user agent, HMAC-hashed IP, and hashed identifiers where needed.

## Design Decisions

### 1. Cookie-based sessions for same-origin

**Decision:** The embedded SPA authenticates via HTTP-only `SameSite=Lax` cookies. The cookie stores an opaque session ID and user ID, while the authoritative `CookieSession` protobuf record lives in `RUNTIME_STATE` with per-key TTL and safe request metadata. The current user record is resolved from projections per request.
**Why:** Cookies are the simplest mechanism for browser SPAs — the browser handles attachment, expiry, and HttpOnly protects against XSS-extracted tokens. Storing the server-side session record in `RUNTIME_STATE` adds revocation for logout, password changes/resets, and account deletion while staying within Chatto's NATS-backed runtime-state model. WebSocket auth comes from the browser sending the cookie with the upgrade request. See ADR-017 and ADR-036.
**Tradeoff:** Non-browser clients can't use cookies. The bearer token path exists for them.

### 2. Bearer tokens for cross-origin

**Decision:** Cross-origin clients (multi-instance frontend, CLI tools) authenticate via opaque bearer tokens stored in `RUNTIME_STATE` under HMAC-derived `session.{hmac}` keys. Tokens are validated by KV lookup plus the current user auth generation derived from `EVT`; single-token revocation is one delete, and user-wide cleanup scans `session.*` for records owned by that user.
**Why:** Cookies are scoped to one origin and `SameSite=Lax` blocks them on cross-origin requests. Tokens are origin-agnostic. We chose opaque tokens over JWTs because Chatto still resolves the current user and permissions server-side per request — JWT's "stateless validation" advantage gives little here, while opaque tokens give instant revocation and natural TTL via KV's built-in expiry. See ADR-024.
**Tradeoff:** Tokens stored in `localStorage` are vulnerable to XSS; cookie sessions are not. Cross-origin clients accept this tradeoff in exchange for being able to authenticate at all. Operators must keep `[core].secret_key` stable across restores to preserve active bearer-token sessions.

### 3. Sliding-window TTL for tokens (and cookies)

**Decision:** Each successful token validation rewrites the runtime-state entry with a fresh per-key TTL (default 90 days). Cookie sessions use the same inactivity window but rotate near expiry instead of touching TTL in place.
**Why:** Time-from-creation expiry would surprise users — "you've been logged in for 90 days, time to re-auth, even though you've been using the app daily". Sliding-window means active users stay logged in indefinitely; only genuinely inactive sessions expire.
**Tradeoff:** A long-stolen token stays valid until it lapses, gets explicitly revoked, or the user's password lifecycle advances the auth cutoff. Operators concerned about shorter compromise windows can lower the TTL.

### 4. WebSocket auth at HTTP upgrade

**Decision:** For cookie clients, authentication happens at the HTTP upgrade handshake. For bearer-token clients, the token is validated from the `connectionParams` payload during the upgrade. By the time the WS is open, the user is already authenticated.
**Why:** Doing auth inside the WS protocol (a `connection_init` payload exchange) adds round-trips and creates a window where the WS is open but not authenticated — easy to misuse, easy to leave open by accident. Upgrade-time auth is atomic. See ADR-017.
**Tradeoff:** Bearer-token WebSocket clients have to deliver the token via `connectionParams` (a graphql-ws feature). Standard pattern, well-supported by libraries.

### 5. Per-request user resolution, no in-session caching

**Decision:** Even though the session stores a user ID, the user record is resolved from the current projections on every request and every WebSocket GraphQL handler.
**Why:** Caching the user in the session would mean serving stale data (display name, roles) across requests. Users expect profile, role, and deletion changes to be immediate; projection-backed resolution gives the current view while keeping sessions small. Dataloaders batch within a single request to prevent fan-out.
**Tradeoff:** Each request still performs server-side user and permission resolution. At Chatto's volume, this is far below noise.

### 6. Cookie auth unchanged when token auth was added

**Decision:** ADR-024 added bearer tokens as a *parallel* path rather than replacing cookies. The auth middleware checks the `Authorization` header first and falls back to the cookie.
**Why:** Existing deployments don't need migration. The embedded SPA keeps working unchanged. Multi-instance frontends and CLI tools get tokens. Both shapes coexist.
**Tradeoff:** Two auth code paths to maintain. They share most logic (user load, middleware injection); only the source of the user ID differs.

### 7. Server version in `connection_ack` for deploy detection

**Decision:** The WebSocket `connection_ack` payload includes the server's binary version. The frontend stores it and prompts the user to refresh when a newer version is detected mid-session.
**Why:** Without it, users get subtle errors when a deployed schema change lands but their old client is still connected. A "the server has been upgraded, please refresh" toast handles it explicitly.
**Tradeoff:** The frontend has to handle the toast and the user has to act on it. Considered acceptable for the rare deployment-during-session case.

### 8. OAuth tokens delivered via query parameter

**Decision:** OAuth callbacks redirect to the frontend with `?token=…` in the URL.
**Why:** The simplest delivery mechanism. The browser hands the token to the frontend; the frontend stores it (or sets up its cookie session) and replaces the URL to drop the parameter from history.
**Tradeoff:** The token briefly appears in browser history and server access logs. Acceptable for v1; a code-exchange flow can be added later if needed. See ADR-024.

### 9. EVT audit facts without raw secrets

**Decision:** Authentication workflows append durable audit facts to `EVT`, but token bodies, links, passwords, auth codes, raw IP addresses, raw redirect URIs, and raw login/email identifiers stay out of the event log. Successful user-scoped facts live on `evt.user.{userId}`; anonymous/server-wide facts such as registration-link issuance and failed login attempts live on `evt.auth.server`.
**Why:** `EVT` is Chatto's durable audit trail as well as the event-sourcing stream. Operators need to answer "what happened?" for sensitive workflows, but the audit log must not become a secondary secret store.
**Tradeoff:** Failed-login and unknown-code exchange attempts intentionally do not reveal whether the submitted identifier or code matched an account. Admins get timing, request metadata, and stable hashes for known-user workflows, not raw credential guesses.

### 10. Short-lived auth codes in runtime state

**Decision:** OAuth authorization codes live in `RUNTIME_STATE` as HMAC-derived `grant.{hmac}` records with a 5-minute per-key TTL and are deleted on exchange attempt.
**Why:** Authorization codes are short-lived, single-use runtime credentials. They need restart survival and TTL enforcement, but they are not domain history and must not be copied into `EVT`.
**Tradeoff:** The returned authorization code remains opaque and unchanged for clients, but the stored key is not human-recoverable from a backup without `[core].secret_key`.

## Permissions

Authentication itself doesn't have a permission gate (you're either authenticated or not). After authentication, downstream actions are gated by the permissions described in FDR-001.

## Related

- **ADRs:** ADR-017 (cookie-session auth for WebSocket), ADR-024 (opaque bearer tokens for cross-origin auth), ADR-025 (multi-instance client architecture), ADR-036 (runtime state in `RUNTIME_STATE`)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle)

## Open Questions

- A "revoke all tokens for this user" admin affordance. Core supports revoke-all for password/account lifecycle flows, but there is no dedicated admin UI action yet.
- A code-exchange OAuth callback flow to keep the token out of the URL/history. Not currently planned.
