# FDR-023: Authentication & Sessions

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Chatto authenticates users via two parallel mechanisms: HTTP-only cookie sessions for the embedded SPA (same-origin) and opaque bearer tokens for cross-origin clients (multi-instance frontends, CLI tools, future mobile apps). Login flows include classic password login, OAuth providers, and a bootstrap path for first-boot operator setup.

## Behavior

- **Login** — users sign in with login + password on a `/login` page. The page is also used for redirect-after-signup.
- **OAuth login** — operators can configure OAuth providers (e.g., Google). The login page shows provider buttons; clicking takes the user through the standard authorization-code flow.
- **Cookie session** — on successful auth from the embedded SPA, the server issues an HTTP-only, SameSite=Lax cookie with a 90-day expiry. The cookie carries the user ID; the server loads the user from KV per request.
- **Bearer token** — every authentication endpoint also issues an opaque token (format: `cht_AT` + 14-char NanoID). Cross-origin clients store it (usually in `localStorage`) and send it as `Authorization: Bearer …` on HTTP requests and `connectionParams.token` on graphql-ws upgrades.
- **WebSocket auth** — for the embedded SPA, the cookie is automatically attached to the WebSocket upgrade and the user is authenticated before the WS handshake completes. For cross-origin clients, the token in `connectionParams` is checked at upgrade time.
- **Logout** — for cookie sessions: the server clears the session and the SPA does a hard reload. For tokens: the client removes the token from `localStorage`; optionally the server revokes the token by deleting its KV key.
- **Session refresh** — the cookie TTL gets refreshed as the user actively uses the app (including on static file requests). Bearer tokens follow a sliding-window TTL — each successful validation re-puts the KV entry to extend the TTL.
- **Server version handshake** — the WebSocket `connection_ack` payload includes the server's version. The frontend uses this to detect deployed-version drift and prompt the user to refresh.

## Design Decisions

### 1. Cookie-based sessions for same-origin

**Decision:** The embedded SPA authenticates via HTTP-only `SameSite=Lax` cookies. The session stores only the user ID; the user record itself is loaded from KV per request.
**Why:** Cookies are the simplest mechanism for browser SPAs — the browser handles attachment, expiry, and HttpOnly protects against XSS-extracted tokens. WebSocket auth comes for free because the browser sends the cookie with the upgrade request. See ADR-017.
**Tradeoff:** Non-browser clients can't use cookies. The bearer token path exists for them.

### 2. Bearer tokens for cross-origin

**Decision:** Cross-origin clients (multi-instance frontend, CLI tools) authenticate via opaque bearer tokens stored in NATS KV. Tokens are validated by KV lookup; revocation is one delete.
**Why:** Cookies are scoped to one origin and `SameSite=Lax` blocks them on cross-origin requests. Tokens are origin-agnostic. We chose opaque tokens over JWTs because Chatto already does a per-request KV lookup to load the user — JWT's "stateless validation" advantage gives nothing here, while opaque tokens give instant revocation and natural TTL via KV's built-in expiry. See ADR-024.
**Tradeoff:** Tokens stored in `localStorage` are vulnerable to XSS; cookie sessions are not. Cross-origin clients accept this tradeoff in exchange for being able to authenticate at all.

### 3. Sliding-window TTL for tokens (and cookies)

**Decision:** Each successful token validation re-puts the KV entry to refresh its TTL (default 90 days). Cookies are similarly refreshed on every static-file request.
**Why:** Time-from-creation expiry would surprise users — "you've been logged in for 90 days, time to re-auth, even though you've been using the app daily". Sliding-window means active users stay logged in indefinitely; only genuinely inactive sessions expire.
**Tradeoff:** A long-stolen token stays valid until it lapses or gets explicitly revoked. Operators concerned about this can lower the TTL or implement a "revoke all tokens for user" action (not currently exposed — see ADR-024).

### 4. WebSocket auth at HTTP upgrade

**Decision:** For cookie clients, authentication happens at the HTTP upgrade handshake. For bearer-token clients, the token is validated from the `connectionParams` payload during the upgrade. By the time the WS is open, the user is already authenticated.
**Why:** Doing auth inside the WS protocol (a `connection_init` payload exchange) adds round-trips and creates a window where the WS is open but not authenticated — easy to misuse, easy to leave open by accident. Upgrade-time auth is atomic. See ADR-017.
**Tradeoff:** Bearer-token WebSocket clients have to deliver the token via `connectionParams` (a graphql-ws feature). Standard pattern, well-supported by libraries.

### 5. Per-request user load, no in-session caching

**Decision:** Even though the session stores a user ID, the user record is loaded from KV on every request (and every WS frame's GraphQL handler).
**Why:** Caching the user in the session would mean serving stale data (display name, roles) across requests. Users expect their profile updates to be immediate; per-request loads guarantee that with negligible cost (KV is memory-cached internally). Dataloaders batch within a single request to prevent fan-out.
**Tradeoff:** A per-request KV `Get`. At Chatto's volume, this is far below noise.

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

## Permissions

Authentication itself doesn't have a permission gate (you're either authenticated or not). After authentication, downstream actions are gated by the permissions described in FDR-001.

## Related

- **ADRs:** ADR-017 (cookie-session auth for WebSocket), ADR-024 (opaque bearer tokens for cross-origin auth), ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle)

## Open Questions

- A "revoke all tokens for this user" affordance for admins. Currently tokens are revoked one at a time by KV key. Useful in the case of a compromised user.
- A code-exchange OAuth callback flow to keep the token out of the URL/history. Not currently planned.
