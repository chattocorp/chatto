# ADR-024: Opaque Bearer Tokens for Cross-Origin Authentication

**Date:** 2026-03-03

**Status:** Partially superseded by [ADR-071](ADR-071-cimd-identified-open-oauth-clients.md), which replaces origin allow-list client registration with CIMD identity and exact callback binding, and [ADR-079](ADR-079-renewable-bearer-sessions.md), which replaces the sliding human bearer-token lifecycle with renewable sessions. The opaque bearer-token decision remains current.

**Supersedes:** Partially extends [ADR-017](ADR-017-cookie-session-auth-for-websocket.md) (same-origin cookie auth remains; this adds a parallel path)

## Context

ADR-017 established cookie-based sessions as the sole authentication mechanism. This works well for the embedded SPA served from the same origin, but cannot support cross-origin clients because:

- `HttpOnly` cookies can't be read or set by JavaScript on a different origin
- `SameSite=Lax` blocks cross-origin POST requests from sending cookies
- The cookie signing secret is instance-specific

To enable a multi-instance client — where a single frontend connects to multiple Chatto backends — we need an authentication mechanism that works across origins.

### Options considered

**JWT (JSON Web Tokens):**
- Self-contained (no server-side lookup needed for validation)
- Standard format with broad library support
- Requires key rotation, clock synchronization, and a blocklist for revocation
- Chatto already performs a KV lookup per request to load the user, so JWT's "no server lookup" advantage provides no real benefit

**Opaque tokens in NATS KV:**
- Simple random strings stored as keys in a KV bucket
- Instant revocation (delete the key)
- Automatic expiry via NATS KV's built-in TTL
- Consistent with the existing storage model (no new infrastructure)
- Requires a KV lookup per request — but we already do one anyway for the user

## Decision

Use opaque bearer tokens stored in NATS KV. Tokens are issued alongside existing cookie sessions (not replacing them) on password, registration, and trusted OAuth code-exchange authentication flows. Clients authenticate via the `Authorization: Bearer <token>` HTTP header for HTTP API requests, and via the realtime websocket token field for live-event delivery.

**2026-05 update:** bearer token records now live in `RUNTIME_STATE` under HMAC-derived `session.{hmac}` keys with per-key TTL. The HMAC input is `session\0{token}` keyed by `[core].secret_key`, so backups can preserve sessions without containing raw bearer-token values.

**Token format:** bearer access credentials use the recognizable `cht_AT`
prefix and remain opaque to clients. ADR-079 replaces the original random
NanoID body with a purpose-separated HMAC value bound to one renewable-session
generation.

**Token lifecycle:** ADR-079 is authoritative. Human bearer authentication now
uses 15-minute access tokens backed by fixed-expiry `session.{hmac}` records
and a rotating refresh credential backed by a stable
`renewable_session.{hmac}` authority with an automatically advancing session
window.
Deleting that authority revokes every access generation. Existing user auth
generation and OAuth-client policy checks remain in force. Issuance and
explicit revocation append safe audit facts to `EVT`; raw access and refresh
credentials are never copied into runtime values or the event log.

**Auth middleware priority:**
1. Check `Authorization: Bearer <token>` header → validate token → load user
2. Fall back to the session cookie only when the request has no browser
   `Origin` header or its origin exactly matches `webserver.url`

**OAuth authorization for cross-origin Chatto clients:**
- Clients start at `/oauth/authorize` with a CIMD URL `client_id`, `response_type=code`, PKCE `code_challenge`, and an exact callback `redirect_uri`. Chatto Desktop uses its built-in client ID instead of a CIMD URL.
- Chatto resolves the client metadata and accepts only an exact redirect URI declared by that identified public client.
- The first authorization for a client shows the user a consent screen. Approval is remembered per user + client ID through durable user EVT facts; denial is also recorded as an audit fact.
- The callback receives a short-lived authorization code, not a bearer token. The client exchanges the code and PKCE verifier at `/oauth/token`.
- Auth codes are stored as HMAC-derived `grant.{hmac}` runtime-state keys, bind the client ID and exact callback, and are deleted on exchange attempt.

## Consequences

- **Cross-origin clients become possible**: Clients that can obtain a bearer token through a trusted OAuth redirect or another authentication flow can authenticate with an HTTP header. This unblocks the multi-instance client epic without trusting arbitrary web origins.
- **Cookie auth stays first-party**: The embedded SPA continues to use its cookie when needed, but a browser origin cannot inject that ambient credential into a cross-origin HTTP or realtime request.
- **Renewal is a separate lifecycle concern**: ADR-079 adds rotating refresh
  credentials and lost-response recovery while retaining the opaque access
  token and server-side revocation properties selected here.
- **Instant revocation**: Deleting a KV key immediately invalidates the token. No blocklist management or "wait for JWT expiry" window.
- **One KV lookup per request**: Token validation requires a `Get` on `RUNTIME_STATE`, but this is negligible given we already do a user load per authenticated request.
- **No reverse index**: cookie cleanup scans `session.*`, while renewable bearer cleanup scans stable `renewable_session.*` authorities and matches the stored user ID. The revocation guarantee comes from the credential's stored auth generation being compared to the current user auth generation, so concurrent issuance cannot survive by missing either scan. A secondary index can be added later if credential counts make scans too expensive.
- **No origin setup**: Chatto permits cross-origin HTTP and realtime transport without credentialed CORS. Exact callback trust comes from CIMD or a built-in client registration instead of operator-managed origin lists.
- **Open client ecosystem**: Any version-compatible public client may connect once it publishes valid CIMD metadata and the user consents. Stable client IDs support consent, audit, and future administrative policy without a preregistration table.
