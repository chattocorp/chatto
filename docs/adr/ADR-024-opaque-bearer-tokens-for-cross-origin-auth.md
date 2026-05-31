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

Use opaque bearer tokens stored in NATS KV. Tokens are issued alongside existing cookie sessions (not replacing them) on all authentication endpoints. Clients authenticate via the `Authorization: Bearer <token>` HTTP header for GraphQL queries/mutations, and via `connectionParams.token` in the graphql-ws `connection_init` payload for WebSocket subscriptions.

**2026-05 update:** bearer token records now live in `RUNTIME_STATE` under HMAC-derived `session.{hmac}` keys with per-key TTL. The HMAC input is `session\0{token}` keyed by `[core].secret_key`, so backups can preserve sessions without containing raw bearer-token values.

**Token format:** `cht_AT` prefix + 14-character NanoID (20 characters total, e.g. `cht_ATa1B2c3D4e5F6G7`). The `cht_` prefix makes tokens recognizable in logs and password managers; the `AT` type prefix follows the existing NanoID convention from ADR-022.

**Token lifecycle:**
- Created on login, registration, bootstrap, and OAuth callback
- Validated by looking up the HMAC-derived `session.{hmac}` key in `RUNTIME_STATE` and reading the stored user ID
- Revoked by deleting the key (idempotent)
- Auto-expired via NATS KV per-key TTL (default 90 days, configurable via `auth.token_ttl`)

**Auth middleware priority:**
1. Check `Authorization: Bearer <token>` header → validate token → load user
2. Fall back to session cookie (existing behavior, unchanged)

## Consequences

- **Cross-origin clients become possible**: Any client that can send an HTTP header can authenticate, regardless of origin. This unblocks the multi-instance client epic.
- **Cookie auth is unchanged**: The embedded SPA continues to work exactly as before. No migration needed for existing deployments.
- **No token refresh complexity**: Long-lived tokens with server-side TTL are simple. If a token expires, the client re-authenticates. No refresh token dance.
- **Instant revocation**: Deleting a KV key immediately invalidates the token. No blocklist management or "wait for JWT expiry" window.
- **One KV lookup per request**: Token validation requires a `Get` on `RUNTIME_STATE`, but this is negligible given we already do a user load per authenticated request.
- **No reverse index**: v1 does not support "revoke all tokens for user". This keeps the implementation simple. If needed later, a prefix-scan or secondary index can be added.
- **OAuth token delivery**: OAuth callbacks append `?token=...` to the redirect URL. This is simple but means the token briefly appears in browser history and server logs. For v1 this is acceptable; a more secure code-exchange flow can be added later if needed.
