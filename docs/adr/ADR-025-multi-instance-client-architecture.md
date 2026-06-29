# ADR-025: Multi-Instance Client Architecture

## Status

Accepted

## Context

Chatto's frontend was originally designed as a single-instance client — the SPA was always served by the Chatto instance it connected to, and all state (auth, spaces, rooms, notifications) was implicitly scoped to that one server.

Users wanted to connect to multiple Chatto instances from a single client (similar to how Discord or Slack allow multiple workspaces). This required rethinking how the frontend manages authentication, state, and routing.

## Decision

### Instance-Agnostic UI

The frontend is instance-agnostic by default. It doesn't assume it's served by a Chatto instance. Instead:

1. **Probe-based origin detection**: On init, call `ServerDiscoveryService.GetServer` on the current origin. If it responds, auto-register the origin as an instance. If it fails (static hosting), skip.
2. **No `isHome` flag**: The origin instance is identified by comparing `instance.url` to `window.location.origin` at runtime — no stored flag.
3. **Bearer-first client auth**: The client stores opaque bearer tokens in `localStorage` for every authenticated instance, including the origin when direct login or registration returns a token. Cookie auth remains as an origin-only fallback for compatibility flows that have not yet handed the SPA a bearer token.

Bearer tokens are only handed to API clients that need to authenticate
ConnectRPC, realtime WebSocket, or direct HTTP API traffic. The Service Worker
asset proxy receives server IDs, base URLs, and hidden ticketed asset targets,
but not API bearer tokens; virtual asset fetches use the asset ticket target and
same-origin cookie fallback instead.

### Unified Registry + State

`InstanceRegistry` owns both registration data (`RegisteredInstance[]`) and per-instance state stores (`SvelteMap<string, InstanceStateStore>`). Registration and store creation are atomic — when an instance is added, its store exists immediately. This eliminates the race condition where `$derived` expressions see a registered instance but can't find its store.

### URL-Based Instance Routing

The URL is the sole source of truth for which instance is active:

- `-` segment = origin instance
- Hostname segment = remote instance (e.g., `chat.example.com`)

The `[instanceId]/+layout.svelte` resolves the segment and provides the instance ID via Svelte context. No mutable "active instance" singleton.

### Per-Instance Permissions

Each server state store has permission and viewer-capability state loaded from ConnectRPC viewer/server-state APIs. This lets the UI show only actions the viewer can perform on the selected server.

### Sliding Window Token Expiry

Bearer tokens use NATS KV TTL (default 90 days). Each successful `ValidateAuthToken` re-puts the entry to reset the TTL. Tokens expire after the configured duration of *inactivity*, not from creation time. Active users are never logged out.

## Consequences

### Positive

- Users can connect to multiple Chatto instances from one client
- The SPA can be served statically (CDN) without a Chatto backend
- No special-casing for "home" vs "remote" — all instances use the same code paths
- Token sliding window means active users never get surprise logouts

### Negative

- Registered-instance bearer tokens in `localStorage` are vulnerable to XSS (cookie auth is not)
- This makes XSS prevention part of the auth boundary. The shipped frontend sets
  a report-only CSP with Trusted Types reporting so deployments can surface
  dangerous script and DOM-sink patterns before policy enforcement is viable for
  the multi-server client.
- `ServerDiscoveryService.GetServer` is the only ConnectRPC endpoint with unconditional wildcard CORS — rich data needed pre-registration must go there, not in authenticated ConnectRPC calls
- Separately hosted multi-instance frontends must be listed explicitly in each remote server's `webserver.oauth_redirect_origins` or exact `webserver.allowed_origins` before OAuth authorization codes can redirect back to them; wildcard CORS does not imply OAuth redirect trust. `oauth_redirect_origins = ["*"]` exists only as a temporary controlled-alpha escape hatch.
- Users approve the first OAuth authorization for each trusted client origin; Chatto remembers that consent per user + origin instead of relying on an operator-managed OAuth client registry
- The probe is async for unauthenticated users, so the origin may not be registered by the time the first render completes

### Trade-offs

- During the transition, cookie and token auth still create two disconnect flows: token failures remove the registered credential, while origin cookie fallback can still require server-side logout + hard reload for compatibility paths.
- SvelteMap for the store map enables reactive `$derived` reads but requires careful separation of imperative writes (`addInstance`) from pure reads (`getStore`)
