# ADR-017: Cookie-Session Authentication Propagated to WebSocket

**Date:** 2026-03-01

## Context

Chatto's frontend is a browser SPA that communicates via GraphQL queries, mutations, and subscriptions. Subscriptions use WebSocket (graphql-ws protocol). The WebSocket upgrade is an HTTP request, which means the browser automatically includes cookies.

Authentication approaches for WebSocket:

- **Bearer token in `connection_init` payload**: Client sends a JWT or session token in the WebSocket init message. Common in mobile/multi-client architectures but requires the client to manage tokens explicitly.
- **Cookie-based session on HTTP upgrade**: The browser sends the session cookie with the upgrade request. The server authenticates during the upgrade handshake, before the WebSocket is established.

## Decision

Use cookie-based sessions (90-day expiry, `HttpOnly`, `SameSiteLax`) for the embedded browser SPA. The session stores `user_id`; the current user is resolved server-side and injected into request context by HTTP middleware. For WebSocket connections, the session cookie is sent with the HTTP upgrade request, so the user is already authenticated before the WebSocket handshake completes.

The WebSocket `InitFunc` reads the authenticated user from context and creates a fresh `context.Background()` with only the user re-injected — dataloaders and other request-scoped state are deliberately stripped. The `connection_ack` payload includes the server version for frontend upgrade detection.

## Consequences

- **Zero client-side token management**: The browser handles cookie storage, expiry, and attachment to requests automatically. No token refresh logic in the frontend.
- **WebSocket auth is implicit**: There's no `connection_init` authentication payload. The user is authenticated before the WS protocol even starts. This simplifies the subscription client.
- **Non-browser clients use bearer tokens**: CLI tools, bots, multi-instance frontends, and future mobile apps can use opaque bearer tokens instead of cookies. Cookie sessions remain the same-origin browser path.
- **Session refresh on static file requests**: The cookie TTL is refreshed when the server serves static frontend files (`refreshSessionIfAuthenticated`), preventing cookie expiry during passive browsing sessions.
- **Server version in `connection_ack`**: The frontend uses this to detect when the server has been upgraded and prompt users to refresh. This is a lightweight deployment coordination mechanism.
