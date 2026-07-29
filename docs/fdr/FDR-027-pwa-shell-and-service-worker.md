# FDR-027: PWA Shell & Service Worker

**Status:** Active
**Last reviewed:** 2026-07-29

## Overview

Chatto ships a service worker so the installed web app can handle push notifications and reuse frontend assets across launches. The worker caches SvelteKit build and static PWA assets only after the browser requests them. The web manifest stays network-only because the server may generate it from current public server branding. The worker deliberately does not cache chat data, API responses, live-event traffic, or protected uploaded asset bodies.

Offline launches are not guaranteed. A previously visited app shell may remain available opportunistically, but the PWA expects a network connection for normal use.

Reconnect catch-up is owned by the foreground web app, not the service worker. When a controlled PWA tab wakes or reconnects, server-scoped stores refetch projected ConnectRPC state and the room UI refetches the currently viewed room/thread window. The worker must not cache or replay messages, API responses, or live-event traffic.

## Behavior

- The service worker is registered by SvelteKit in production builds.
- Installing or updating the worker does not download frontend assets.
- On activate, old Chatto shell caches are deleted and the new worker claims open clients.
- Known build and static PWA assets are cached when requested and served cache-first afterward.
- The served web manifest uses the server name as the installed app name. Its icons, along with favicon and Apple touch icon metadata, use the uploaded server logo when one exists and fall back to bundled Chatto icons otherwise.
- Same-origin navigations are network-first. Successful navigations update the cached SPA shell without another request; that shell is used as a best-effort fallback when the network fails.
- API, auth, OAuth, webhook, uploaded-asset, dynamic branding metadata, non-GET, and cross-origin requests are network-only.
- Protected uploaded asset loads use direct signed asset URLs owned by the foreground app. The worker does not receive registered-server API bearer tokens, does not proxy asset requests, and does not cache protected asset bodies.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Push dismiss payloads still close matching visible notifications on the device.

## Design Decisions

### 1. Shell-only caching

**Decision:** Cache static frontend assets only after the browser requests them. Successful navigations may seed a shell fallback, but installing the worker does not precache the build. The web manifest remains network-only.
**Why:** Chatto is a real-time chat app that requires the network for useful state. Downloading every route, lazy chunk, font, and stylesheet during service-worker installation makes startup contend with assets the user may never need.
**Tradeoff:** Offline launches are best-effort and may fail if the necessary shell assets have not already been requested. In exchange, first load and worker updates avoid a large background request burst.

### 2. Versioned cache names

**Decision:** Shell caches include the SvelteKit app version in their name.
**Why:** A deploy can replace hashed JavaScript and CSS chunks. Versioned cache names let the new worker populate a fresh shell cache and delete older shell caches during activation.
**Tradeoff:** A user may briefly store assets from two shell versions during update. Old versioned caches are removed when the new worker activates.

### 3. SvelteKit owns registration

**Decision:** The frontend relies on SvelteKit's production service-worker registration instead of registering manually from the push-notification setup component.
**Why:** The service worker is now useful even when Web Push is not enabled. Registration should be tied to the PWA shell, not to push settings.
**Tradeoff:** Production users get the service worker whenever the app includes one. The worker's fetch policy is conservative to make that safe.

### 4. Protected assets stay outside the worker

**Decision:** Protected uploaded assets are loaded through direct signed asset URLs and refreshed by foreground components when they approach expiry or fail to load. The service worker treats uploaded assets as network-only and never proxies or caches their bodies.
**Why:** The asset tickets and `AssetService` refresh flow are the actual reliability and authorization mechanism. Keeping asset routing out of the worker removes hidden worker/client state and keeps the service worker focused on shell availability and notifications.
**Tradeoff:** Ticketed asset URLs are visible in normal page markup. Their exposure is bounded by the ticket expiry and by the server's room-membership check on every fetch.

### 5. Install metadata follows server branding

**Decision:** The HTTP frontend server generates the web manifest from the bundled manifest, uses the current server name for the installed app name, and swaps in transformed server-logo URLs for install icons when a logo is configured. Stable favicon and Apple touch icon endpoints redirect to purpose-sized transforms of the current server logo, or to the bundled Chatto icons when no logo is configured.
**Why:** Self-hosted servers should install with their own visible identity without requiring a custom frontend build.
**Tradeoff:** Browsers decide when to refresh installed PWA metadata and may cache it aggressively, so existing installs or tabs may keep the previous name or icon until the browser revalidates the metadata or the user reinstalls the app.

## Related

- **ADRs:** ADR-043 (client-shell internationalization), ADR-047 (direct ticketed asset URLs)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications)
