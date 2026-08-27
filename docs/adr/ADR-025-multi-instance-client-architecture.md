# ADR-025: Multi-Server Client Architecture

**Date:** 2026-03-20

**Updated:** 2026-08-25

## Status

Partially superseded by [ADR-071](ADR-071-cimd-identified-open-oauth-clients.md),
which replaces the origin allow-list client-registration requirement, and
[ADR-074](ADR-074-keep-server-catalogue-device-local.md), which makes the
server catalogue device-local. [ADR-079](ADR-079-renewable-bearer-sessions.md)
replaces the sliding bearer-token lifetime. The multi-server client
architecture remains current.

## Context

Chatto's frontend was originally designed as a single-server client — the SPA was always served by the Chatto server it connected to, and all state (auth, rooms, notifications) was implicitly scoped to that one server.

Users wanted to connect to multiple Chatto servers from a single client (similar to how Discord or Slack allow multiple workspaces). This required rethinking how the frontend manages authentication, state, and routing.

## Decision

### Server-Agnostic UI

The frontend is server-agnostic by default. It doesn't assume it is served by a Chatto server. Instead:

1. **Probe-based origin detection**: On init, call `chatto.discovery.v1.ServerDiscoveryService.GetServer` on the current origin. If it responds, auto-register the origin as a server. If it fails (static hosting), skip.
2. **No `isHome` flag**: The origin server is identified by comparing `server.url` to `window.location.origin` at runtime — no stored flag.
3. **Cookie-only origin auth**: The client uses the HttpOnly cookie for the
   server that serves the SPA. Dedicated browser authentication does not issue
   bearer credentials for that server. During migration, the client revokes
   old origin bearer authority before it removes the local credentials. Remote
   servers use persisted renewable bearer sessions obtained through Chatto
   OAuth.

Bearer tokens are only handed to API clients that need to authenticate
ConnectRPC, realtime WebSocket, or direct HTTP API traffic. Browser media
elements do not receive bearer tokens; remote attachment media uses direct
per-user asset access tickets on stable asset URLs instead.

### Server Catalogue, Sessions, and Retained State

ADR-064 supersedes this decision's original unified registration-and-session
model. Public server catalogue metadata and device-local authentication are
separate reactive state owners. `ServerRegistry` composes them with per-server
state stores and connections. Catalogue registration and store creation remain
atomic: when a server is added, its retained store exists immediately.

The persisted `localStorage` slot intentionally remains named `instances`, and
its combined record remains a compatibility adapter. It is split into catalogue
and session state at runtime. Authentication fields migrate into independently
keyed per-server records; combined-record saves merge those authoritative
fields so a stale tab cannot restore an older rotated credential generation.

The catalogue and sessions are device-local under ADR-074. A server can remain
known while signed out, and selecting it starts the normal Chatto OAuth flow.
The existing `chatto:instances` compatibility record accepts former provenance
fields but rewrites every registration into the device-local catalogue. The
frontend has no global identity-provider session or synchronized catalogue.

### URL-Based Server Routing

The URL is the sole source of truth for which server is active:

- `-` segment = origin server
- Hostname segment = remote server (e.g., `chat.example.com`)

The `[serverId]/+layout.svelte` resolves the segment and provides the server ID via Svelte context. No mutable "active server" singleton.

### Per-Server Permissions

Each server state store has permission and viewer-capability state loaded from ConnectRPC viewer/server-state APIs. This lets the UI show only actions the viewer can perform on the selected server.

### Renewable Bearer Lifetime

Human bearer sessions use short fixed-lifetime access tokens and rotating
refresh credentials. The frontend serializes rotation, refreshes before access
expiry, and advances the renewable-session window without user action. Origin
cookie sessions renew one stable handle in the final quarter of the current
credential window. ADR-079 and ADR-081 own the detailed renewal, recovery,
revocation, and expiry contract.

## Consequences

### Positive

- Users can connect to multiple Chatto servers from one client
- The SPA can be served statically (CDN) without a Chatto backend
- No special-casing for "home" vs "remote" — all servers use the same code paths
- Short access lifetimes bound an access token stolen without its refresh
  credential, while background rotation avoids frequent interactive login

### Negative

- Remote-server bearer credentials in `localStorage` are vulnerable to XSS
  (origin cookie auth is not)
- This makes XSS prevention part of the auth boundary. The shipped frontend
  enforces a CSP. It uses build-time hashes for inline bootstrap scripts and
  does not allow general inline scripts.
- All public HTTP and realtime entry points permit browser transport from any syntactically valid origin without credentialed CORS. Cross-origin clients must present bearer tokens; ambient cookie credentials remain same-origin only.
- Separately hosted frontends publish a CIMD document and send its URL as `client_id`. Chatto validates the exact callback from that document, while Desktop uses its fixed built-in registration.
- Users approve the first OAuth authorization for each client; Chatto remembers consent per user + stable client ID without an operator-managed registration table.
- Signing in to each Chatto server remains a separate authorization and creates
  a device-local session.
- The probe is async for unauthenticated users, so the origin may not be registered by the time the first render completes

### Trade-offs

- Bearer refresh failure preserves the route and marks only that server as
  requiring explicit authentication, while an invalid origin cookie session
  can still require server-side logout plus a hard reload. The two
  presentations retain distinct disconnect flows.
- SvelteMap for the store map enables reactive `$derived` reads but requires careful separation of imperative writes (`addServer`) from pure reads (`getStore`)
