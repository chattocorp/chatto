# FDR-027: PWA Shell & Service Worker

**Status:** Active
**Last reviewed:** 2026-07-09

## Overview

Chatto ships a service worker so the installed web app can launch reliably and handle push notifications. The worker caches the SPA fallback shell and SvelteKit build assets during install, then caches static PWA assets when the browser actually requests them. The web manifest stays network-only because the server may generate it from current public server branding. The worker deliberately does not cache chat data, API responses, live-event traffic, or protected uploaded asset bodies.

Offline support means the app can open and show its normal disconnected state instead of the browser's generic offline page. It does not mean offline message history, offline search, or an outbox for composing messages while disconnected.

Reconnect catch-up is owned by the foreground web app, not the service worker. When a controlled PWA tab wakes or reconnects, server-scoped stores refetch projected ConnectRPC state and the room UI refetches the currently viewed room/thread window. The worker must not cache or replay messages, API responses, or live-event traffic.

## Behavior

- The service worker is registered by SvelteKit in production builds.
- On install, the worker caches the SPA fallback shell and SvelteKit build assets required to boot it.
- On activate, old Chatto shell caches are deleted and the new worker claims open clients.
- Known shell assets are served cache-first from the versioned cache; static PWA assets other than the web manifest are cached lazily on first request.
- The served web manifest and Apple touch icon metadata use the uploaded server logo when one exists, falling back to bundled Chatto icons otherwise.
- Same-origin navigations are network-first, falling back to the cached SPA shell only when the network fails.
- API, auth, OAuth, webhook, uploaded-asset, web-manifest, non-GET, and cross-origin requests are network-only.
- Protected uploaded asset loads use direct signed asset URLs owned by the foreground app. The worker does not receive registered-server API bearer tokens, does not proxy asset requests, and does not cache protected asset bodies.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Push dismiss payloads still close matching visible notifications on the device.

## Design Decisions

### 1. Shell-only caching

**Decision:** Cache only the app shell and static PWA assets that do not depend on current server state. Build assets required to boot the shell are cached during install, while static PWA assets are cached lazily after install. The web manifest remains network-only.
**Why:** Chatto is a real-time chat app. Serving stale messages, permissions, assets, or notification state as if they were live would be worse than showing the disconnected state.
**Tradeoff:** Offline launches do not show recent rooms or messages unless the live app already has that state in memory, and full static asset coverage accumulates as the app requests assets. The install manifest may be unavailable while offline, but installed apps already have their manifest metadata. This keeps the data model honest while avoiding install-time requests for icons and unrelated static files.

### 2. Versioned cache names

**Decision:** Shell caches include the SvelteKit app version in their name.
**Why:** A deploy can replace hashed JavaScript and CSS chunks. Versioned cache names let the new worker populate a fresh shell cache and delete older shell caches during activation.
**Tradeoff:** A user briefly stores two shell versions during update. The cached asset set is small, so this is acceptable.

### 3. SvelteKit owns registration

**Decision:** The frontend relies on SvelteKit's production service-worker registration instead of registering manually from the push-notification setup component.
**Why:** The service worker is now useful even when Web Push is not enabled. Registration should be tied to the PWA shell, not to push settings.
**Tradeoff:** Production users get the service worker whenever the app includes one. The worker's fetch policy is conservative to make that safe.

### 4. Protected assets stay outside the worker

**Decision:** Protected uploaded assets are loaded through direct signed asset URLs and refreshed by foreground components when they approach expiry or fail to load. The service worker treats uploaded assets as network-only and never proxies or caches their bodies.
**Why:** The asset tickets and `AssetService` refresh flow are the actual reliability and authorization mechanism. Keeping asset routing out of the worker removes hidden worker/client state and keeps the service worker focused on shell availability and notifications.
**Tradeoff:** Ticketed asset URLs are visible in normal page markup. Their exposure is bounded by the ticket expiry and by the server's room-membership check on every fetch.

### 5. Install metadata follows server branding

**Decision:** The HTTP frontend server generates the web manifest from the bundled manifest and swaps in transformed server-logo URLs for install icons when a logo is configured. The served HTML similarly replaces the Apple touch icon link with a fixed-size server-logo transform.
**Why:** Self-hosted servers should install with their own visible identity without requiring a custom frontend build.
**Tradeoff:** Browsers decide when to refresh installed PWA metadata, so existing installs may keep the previous icon until the browser updates the manifest or the user reinstalls the app.

## Related

- **ADRs:** ADR-043 (client-shell internationalization), ADR-047 (direct ticketed asset URLs)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications)
