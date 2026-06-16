# FDR-023: Authentication & Sessions

**Status:** Active
**Last reviewed:** 2026-06-16

## Overview

Chatto authenticates users via two parallel mechanisms: HTTP-only cookie sessions for the embedded SPA (same-origin) and opaque bearer tokens for cross-origin clients (multi-instance frontends, CLI tools, future mobile apps). Login flows include classic password login, configured external providers, and a bootstrap path for first-boot operator setup.

## Behavior

- **Login** — users sign in with login + password on a `/login` page. The page is also used for redirect-after-signup.
- **External provider login** — operators configure repeated `[[auth.providers]]` entries or equivalent counted `CHATTO_AUTH_PROVIDERS_<index>_<field>` environment variables. Supported provider types are `oidc`, `github`, `gitlab`, `google`, and `discord`. The login page renders buttons from `Server.authProviders`, and provider flows use `/auth/providers/{providerID}` plus `/auth/providers/{providerID}/callback`. OIDC also keeps `/auth/oidc` and `/auth/oidc/callback` as compatibility aliases for older clients and provider registrations.
- **External identity matching** — provider identities are linked through durable `EVT` user facts. OIDC providers use the verified issuer URL plus subject; OAuth-only providers use the configured provider ID plus Goth `UserID`. Email claims are optional profile data and are not used as the primary account match key. If a provider callback returns an identity that is not linked to a Chatto account, the login is rejected until an account creation/linking flow links that identity.
- **Chatto OAuth authorization** — cross-origin Chatto clients use `/oauth/authorize` with PKCE to obtain a short-lived authorization code, then exchange it at `/oauth/token` for an opaque bearer token. Redirect URIs must use this server's configured `webserver.url` origin, an explicit `webserver.oauth_redirect_origins` entry, an exact `webserver.allowed_origins` entry, or a loopback development origin; wildcard CORS does not authorize OAuth redirects. `oauth_redirect_origins = ["*"]` temporarily accepts any valid HTTPS redirect origin for controlled alpha deployments. The first authorization for a trusted redirect origin shows a user consent screen and remembers approval per user + origin.
- **Cookie session** — on successful auth from the embedded SPA, the server issues an HTTP-only, SameSite=Lax cookie with a 90-day expiry. The cookie carries an opaque session ID plus the user ID needed to derive the lookup key; the authoritative `CookieSession` protobuf record lives in `RUNTIME_STATE` under `cookie_session.{userId}.{hmac}` with a per-key TTL. The server validates the KV record and resolves the current user from projections per request.
- **CSRF protection** — cookie-session unsafe non-GraphQL requests must include an `X-CSRF-Token` header matching the readable `chatto_csrf` cookie and the token stored in the signed session, except for public auth/bootstrap endpoints, test endpoints, webhooks, and OAuth token exchange. GraphQL requests do not use the double-submit token; same-origin cookie GraphQL calls carry the non-simple `X-REQUEST-TYPE: GraphQL` header as the API request marker. Bearer-only requests are exempt because bearer credentials are not ambient browser cookies.
- **Bearer token** — every authentication endpoint also issues an opaque token (format: `cht_AT` + 14-char NanoID). Cross-origin clients store it (usually in `localStorage`) and send it as `Authorization: Bearer …` on HTTP requests and `connectionParams.token` on graphql-ws upgrades. The token record lives in `RUNTIME_STATE` as an HMAC-derived `session.{hmac}` key with a per-key TTL.
- **WebSocket auth** — for the embedded SPA, the cookie is automatically attached to the WebSocket upgrade and the user is authenticated before the WS handshake completes. For cross-origin clients, the token in `connectionParams` is checked at upgrade time.
- **Logout** — for cookie sessions: the server deletes the current cookie-session KV record, clears the cookie, and the SPA does a hard reload. For tokens: the client removes the token from `localStorage`; optionally the server revokes the token by deleting its KV key.
- **Session refresh** — cookie-session KV TTL cannot be touched in place, so active sessions are rotated near expiry: the server creates a replacement `CookieSession` record with a fresh TTL, updates the browser cookie, and deletes the old record best-effort. Bearer tokens follow a sliding-window TTL — each successful validation rewrites the `RUNTIME_STATE` entry with a fresh per-key TTL.
- **Password and account lifecycle revocation** — password resets, password changes, and account deletion advance the user's auth generation through durable `EVT` user events. Cookie sessions, bearer tokens, and OAuth authorization codes store the auth generation they were issued against; validation waits the user projection to the current auth-generation events and rejects credentials from older generations. Generation `0` is reserved for pre-field legacy runtime credentials and is upgraded on validation when the credential is not older than the current password event. Revoke-all scans still delete matching `cookie_session.*` and `session.*` records as cleanup.
- **Password reset tokens** — reset links are backed by `RUNTIME_STATE` HMAC-derived `password_reset.{hmac}` records with a 1-hour per-key TTL. Raw reset tokens and links are never written to `EVT` or backup archives.
- **Server version handshake** — the WebSocket `connection_ack` payload includes the server's version. The frontend uses this to detect deployed-version drift and prompt the user to refresh.
- **Auth audit facts** — successful cookie/provider logins, failed password login attempts, logout completion, bearer-token issuance/revocation, OAuth consent decisions, OAuth authorization-code issuance/exchange, registration-code issuance, email-verification-code issuance, password-reset link issuance, and password-reset completion are appended to `EVT` for admin audit-log inspection. Payloads carry safe request metadata only: capped user agent, HMAC-hashed IP, and hashed identifiers where needed.

## Design Decisions

### 1. Cookie-based sessions for same-origin

**Decision:** The embedded SPA authenticates via HTTP-only `SameSite=Lax` cookies. The cookie stores an opaque session ID and user ID, while the authoritative `CookieSession` protobuf record lives in `RUNTIME_STATE` with per-key TTL and safe request metadata. The current user record is resolved from projections per request.
**Why:** Cookies are the simplest mechanism for browser SPAs — the browser handles attachment, expiry, and HttpOnly protects against XSS-extracted tokens. Storing the server-side session record in `RUNTIME_STATE` adds revocation for logout, password changes/resets, and account deletion while staying within Chatto's NATS-backed runtime-state model. WebSocket auth comes from the browser sending the cookie with the upgrade request. See ADR-017 and ADR-036.
**Tradeoff:** Non-browser clients can't use cookies. The bearer token path exists for them.

### 1a. CSRF protection for cookie-authenticated writes

**Decision:** Same-origin cookie-session writes require a CSRF proof unless they target GraphQL, public auth/bootstrap endpoints, test endpoints, webhooks, or OAuth token exchange. The server stores a token in the signed session, mirrors it into a readable `chatto_csrf` cookie for the SPA, and accepts `X-CSRF-Token` when it matches both. GraphQL requests are treated as API traffic instead: the SPA sends `X-REQUEST-TYPE: GraphQL`, a non-simple request marker that cross-site forms cannot send, and does not send the CSRF token. Bearer-only requests skip the CSRF check.
**Why:** Cookie sessions are ambient browser credentials, so another site can try to submit state-changing requests with the user's cookie attached. Browser form CSRF is a poor fit for the GraphQL API surface, and coupling GraphQL to a mirrored CSRF cookie made auth refresh more brittle than necessary. The request marker keeps GraphQL programmatic and cross-origin setup paths simple while still distinguishing API calls from simple browser form posts.
**Tradeoff:** The frontend must attach the CSRF header for protected origin POSTs outside GraphQL. The token is not HttpOnly by design; it is not an auth secret, only a same-origin request proof paired with the signed session. GraphQL clients must send the request marker when relying on cookie auth.

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

### 8. OAuth code exchange and redirect-origin allow-list

**Decision:** Chatto's OAuth authorization endpoint returns a short-lived authorization code to the client's callback; the client exchanges that code plus its PKCE verifier at `/oauth/token` for a bearer token. Redirect URIs are accepted only for trusted origins: the server's own configured `webserver.url` origin, explicit `webserver.oauth_redirect_origins` entries, exact `webserver.allowed_origins` entries, and loopback development origins. The first authorization for a user + redirect origin requires explicit consent; approved origins are remembered through durable `EVT` facts.
**Why:** PKCE keeps bearer tokens out of callback URLs, the redirect allow-list prevents a logged-in user from being tricked into sending an authorization code to an attacker-controlled HTTPS origin, and consent gives the user a human-readable checkpoint before a bearer token can be minted for a trusted client origin. The `allowed_origins = ["*"]` CORS default is intentionally not treated as OAuth trust.
**Tradeoff:** A separately hosted multi-server frontend must be listed explicitly in each server's `webserver.oauth_redirect_origins` or exact `webserver.allowed_origins` before users can connect that server through OAuth, and users see one prompt per trusted client origin. For controlled alpha deployments, `oauth_redirect_origins = ["*"]` temporarily restores broad HTTPS callback acceptance at the cost of weakening redirect-origin protection. We do not use an operator-managed client registry because any compatible Chatto client should be able to connect to any compatible Chatto server once its origin is trusted.

### 8a. External provider registry and identity links

**Decision:** External login providers are configured as repeated `[[auth.providers]]` entries with stable local IDs, curated provider types, labels, client credentials, optional scopes, optional email-scope requests, and provider options reserved for provider-specific expansion. OIDC verification remains on `go-oidc`; OAuth-only providers use Goth for authorization-code exchange and normalized user fetching. Linked identities are durable `EVT` facts on the user aggregate.
**Why:** A provider list lets operators expose more than one login option without hard-coding a single OIDC provider. Keeping the provider ID stable gives OAuth-only providers a local issuer namespace. OIDC uses the verified issuer URL as the durable namespace because the issuer owns subject uniqueness; the configured provider ID is event-time metadata and can change without invalidating existing OIDC links. Matching by issuer/provider plus subject survives provider-side email changes and avoids account takeover through recycled or unverified email addresses.
**Tradeoff:** Existing `[auth.oidc]` TOML configuration is not accepted by this shape; deployments should use `[[auth.providers]]`. Legacy `CHATTO_AUTH_OIDC_*` environment variables are still accepted as a compatibility bridge for one OIDC provider. Account creation/linking for new external identities must explicitly create a link before provider login can succeed.

### 9. EVT audit facts without raw secrets

**Decision:** Authentication workflows append durable audit facts to `EVT`, but token bodies, verification codes, links, passwords, auth codes, raw IP addresses, full redirect URIs, and raw login/email identifiers stay out of the event log. OAuth consent grant/deny facts store the canonical redirect origin in plaintext so users can recognize approved client addresses in future management UI. Successful user-scoped facts live on `evt.user.{userId}`; anonymous/server-wide facts such as registration-code issuance and failed login attempts live on `evt.auth.server`.
**Why:** `EVT` is Chatto's durable audit trail as well as the event-sourcing stream. Operators need to answer "what happened?" for sensitive workflows, but the audit log must not become a secondary secret store.
**Tradeoff:** Failed-login and unknown-code exchange attempts intentionally do not reveal whether the submitted identifier or code matched an account. Admins get timing, request metadata, and stable hashes for known-user workflows, not raw credential guesses.

**OTP guardrails:** Registration and authenticated email-verification OTPs share `RUNTIME_STATE` `email_otp.*` records. Each challenge allows at most ten issued codes and five wrong-code attempts in its 15-minute TTL window; exhaustion blocks fresh codes for that challenge until TTL.

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
