# ADR-024: Opaque Bearer Tokens for Cross-Origin Authentication

**Date:** 2026-03-03

**Supersedes:** Partially extends [ADR-017](ADR-017-cookie-session-auth-for-websocket.md) (cookie auth remains unchanged; this adds a parallel path)

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

**Token format:** `cht_AT` prefix + 14-character NanoID (20 characters total, e.g. `cht_ATa1B2c3D4e5F6G7`). The `cht_` prefix makes tokens recognizable in logs and password managers; the `AT` type prefix follows the existing NanoID convention from ADR-022.

**Token lifecycle:**
- Created on login, registration, bootstrap, and OAuth callback
- Validated by looking up the HMAC-derived `session.{hmac}` key in `RUNTIME_STATE` and reading the stored user ID
- Rejected when the token's stored auth generation differs from the current user auth generation derived from durable user events
- Legacy records with no stored auth generation unmarshal as generation `0`; validation upgrades them to the current generation when their `created_at` is not older than the current password event
- Revoked by deleting the key (idempotent)
- Cleaned up for a whole user by scanning `session.*` records, matching the stored user ID, and deleting each match; password resets, password changes, and account deletion use this path after advancing the user's auth generation

Issuance and explicit revocation append safe audit facts to `EVT` with source/reason and request metadata. The raw bearer token and token-key HMAC are never copied into the event log.
- Auto-expired via NATS KV per-key TTL (default 90 days, configurable via `auth.token_ttl`)

**Auth middleware priority:**
1. Check `Authorization: Bearer <token>` header → validate token → load user
2. Fall back to session cookie (existing behavior, unchanged)

**OAuth authorization for cross-origin Chatto clients:**
- Clients start at `/oauth/authorize` with `response_type=code`, PKCE `code_challenge`, and a callback `redirect_uri`.
- The server only accepts redirect URI origins it trusts: the configured `webserver.url` origin, explicit `webserver.allowed_origins` entries, explicit `webserver.oauth_redirect_origins` entries, and loopback development origins. The wildcard CORS default (`allowed_origins = ["*"]`) does not authorize OAuth redirects.
- `oauth_redirect_origins = ["*"]` is an OAuth-specific temporary escape hatch for controlled alpha deployments: it accepts any otherwise valid HTTPS redirect origin while preserving loopback HTTP/HTTPS development redirects. This reopens the authorization-code exfiltration risk that exact origin trust is meant to reduce, so production deployments should prefer exact origins or a narrow trusted frontend origin.
- The first authorization for a trusted redirect origin shows the user a consent screen. Approval is remembered per user + canonical redirect origin through durable user EVT facts; denial is also recorded as an audit fact.
- The callback receives a short-lived authorization code, not a bearer token. The client exchanges the code and PKCE verifier at `/oauth/token`.
- Auth codes are stored as HMAC-derived `grant.{hmac}` runtime-state keys and are deleted on exchange attempt.

## Consequences

- **Cross-origin clients become possible**: Clients that can obtain a bearer token through a trusted OAuth redirect or another authentication flow can authenticate with an HTTP header. This unblocks the multi-instance client epic without trusting arbitrary web origins.
- **Cookie auth is unchanged**: The embedded SPA continues to work exactly as before. No migration needed for existing deployments.
- **No token refresh complexity**: Long-lived tokens with server-side TTL are simple. If a token expires, the client re-authenticates. No refresh token dance.
- **Instant revocation**: Deleting a KV key immediately invalidates the token. No blocklist management or "wait for JWT expiry" window.
- **One KV lookup per request**: Token validation requires a `Get` on `RUNTIME_STATE`, but this is negligible given we already do a user load per authenticated request.
- **No reverse index**: user-wide cleanup does a `session.*` prefix scan and matches the stored user ID. The revocation guarantee comes from the token's stored auth generation being compared to the current user auth generation, so concurrent issuance cannot survive by missing the scan. A secondary index can be added later if token counts make scans too expensive.
- **OAuth redirect setup**: A separately hosted Chatto frontend must be configured in `webserver.oauth_redirect_origins` or as an exact `webserver.allowed_origins` entry on each server it connects to. This adds an operator step, but prevents malicious sites from using `/oauth/authorize` as a logged-in user's bearer-token minting oracle. During controlled alpha use, `oauth_redirect_origins = ["*"]` can temporarily trade that protection for connectivity.
- **No client registry**: Chatto does not require `client_id` registration for this flow. Any version-compatible Chatto client may connect once its origin is trusted and the user consents.
