# ADR-017: Cookie-Session Authentication Propagated to WebSocket

**Date:** 2026-03-01

## Context

Chatto's frontend is a browser SPA that communicates through HTTP APIs plus a realtime websocket. The WebSocket upgrade is an HTTP request, which means the browser automatically includes same-origin cookies.

Authentication approaches for WebSocket:

- **Bearer token in `connection_init` payload**: Client sends a JWT or session token in the WebSocket init message. Common in mobile/multi-client architectures but requires the client to manage tokens explicitly.
- **Cookie-based session on HTTP upgrade**: The browser sends the session cookie with the upgrade request. The server authenticates during the upgrade handshake, before the WebSocket is established.

## Decision

Use cookie-based sessions (90-day expiry, `HttpOnly`, `SameSiteLax`) for the embedded browser SPA. The session stores `user_id`; the current user is resolved server-side and injected into request context by HTTP middleware. For WebSocket connections, the session cookie is sent with the HTTP upgrade request, so the user is already authenticated before the WebSocket handshake completes.

The realtime WebSocket handler reads the authenticated user from request context and creates connection-scoped state without inheriting request-local caches. The connection acknowledgement includes the server version for frontend upgrade detection.

## Consequences

- **Zero client-side token management**: The browser handles cookie storage, expiry, and attachment to requests automatically. No token refresh logic in the frontend.
- **WebSocket auth is implicit for same-origin cookie clients**: The user is authenticated before the WS protocol even starts. Bearer-token clients use the realtime protocol's token path.
- **Non-browser clients use bearer tokens**: CLI tools, bots, multi-instance frontends, and future mobile apps can use opaque bearer tokens instead of cookies. Cookie sessions remain the same-origin browser path.
- **Session refresh on static file requests**: The cookie TTL is refreshed when the server serves static frontend files (`refreshSessionIfAuthenticated`), preventing cookie expiry during passive browsing sessions.
- **Server version in the connection acknowledgement**: The frontend uses this to detect when the server has been upgraded and prompt users to refresh. This is a lightweight deployment coordination mechanism.
