# FDR-027: PWA Shell & Service Worker

**Status:** Active
**Last reviewed:** 2026-06-20

## Overview

Chatto ships a service worker so the installed web app can launch reliably, handle push notifications, and proxy private asset loads through non-portable same-origin URLs. The worker caches the SPA fallback shell and SvelteKit build assets during install, then caches static PWA assets when the browser actually requests them. It deliberately does not cache chat data, API responses, live-event traffic, or protected uploaded asset bodies.

Offline support means the app can open and show its normal disconnected state instead of the browser's generic offline page. It does not mean offline message history, offline search, or an outbox for composing messages while disconnected.

Reconnect catch-up is owned by the foreground web app, not the service worker. When a controlled PWA tab wakes or reconnects, server-scoped stores refetch projected ConnectRPC state and the room UI refetches the currently viewed room/thread window. The worker must not cache or replay messages, API responses, or live-event traffic.

## Behavior

- The service worker is registered by SvelteKit in production builds.
- On install, the worker caches the SPA fallback shell and SvelteKit build assets required to boot it.
- On activate, old Chatto shell caches are deleted and the new worker claims open clients.
- Known shell assets are served cache-first from the versioned cache; static PWA assets are cached lazily on first request.
- Same-origin navigations are network-first, falling back to the cached SPA shell only when the network fails.
- API, auth, webhook, non-GET, and cross-origin requests are network-only.
- Same-origin virtual asset requests under `/__chatto/assets/{serverId}/...` are resolved by the worker to the registered server's hidden ticketed asset URL. The worker does not receive registered-server API bearer tokens, asks Chatto to stream originals instead of redirecting to S3 for full responses, does not cache protected asset bodies, and keeps media `Range` requests network-only.
- If the browser restarts an idle worker while controlled pages stay open, the worker asks those clients to resend registered servers and virtual asset target mappings before treating a virtual asset as unresolved.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Push dismiss payloads still close matching visible notifications on the device.

## Design Decisions

### 1. Shell-only caching

**Decision:** Cache only the app shell and static PWA assets. Build assets required to boot the shell are cached during install, while static PWA assets are cached lazily after install.
**Why:** Chatto is a real-time chat app. Serving stale messages, permissions, assets, or notification state as if they were live would be worse than showing the disconnected state.
**Tradeoff:** Offline launches do not show recent rooms or messages unless the live app already has that state in memory, and full static asset coverage accumulates as the app requests assets. This keeps the data model honest while avoiding install-time requests for icons, manifests, and unrelated static files.

### 2. Versioned cache names

**Decision:** Shell caches include the SvelteKit app version in their name.
**Why:** A deploy can replace hashed JavaScript and CSS chunks. Versioned cache names let the new worker populate a fresh shell cache and delete older shell caches during activation.
**Tradeoff:** A user briefly stores two shell versions during update. The cached asset set is small, so this is acceptable.

### 3. SvelteKit owns registration

**Decision:** The frontend relies on SvelteKit's production service-worker registration instead of registering manually from the push-notification setup component.
**Why:** The service worker is now useful even when Web Push is not enabled. Registration should be tied to the PWA shell, not to push settings.
**Tradeoff:** Production users get the service worker whenever the app includes one. The worker's fetch policy is conservative to make that safe.

### 4. Virtual private asset URLs

**Decision:** In controlled browser sessions, the frontend renders stable asset URLs through a same-origin Service Worker namespace (`/__chatto/assets/{serverId}/...`) instead of putting the ticketed remote URL directly in markup.
**Why:** Remote-server cookies are not reliable for third-party media loads, while ticketed asset URLs are bearer capabilities if copied. The virtual URL is only useful inside a Chatto client that has the server registration and credentials needed to resolve it.
**Tradeoff:** The worker keeps a hidden mapping from virtual URL to the current ticketed target URL so existing backend transform signing keeps working. API bearer tokens are deliberately not copied into worker asset state, so remote assets depend on that ticketed target mapping instead of a bearer-token fallback. If the worker is not controlling the page, the frontend falls back to the direct ticketed URL. Media `Range` requests are redirected to the hidden target URL rather than cached until the backend grows deliberate Range streaming through Chatto.

## Related

- **ADRs:** ADR-039 (Service Worker virtual asset URLs with ticketed fallback), ADR-043 (client-shell internationalization)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications)
