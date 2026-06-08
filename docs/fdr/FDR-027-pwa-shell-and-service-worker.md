# FDR-027: PWA Shell & Service Worker

**Status:** Active
**Last reviewed:** 2026-06-08

## Overview

Chatto ships a service worker so the installed web app can launch reliably and handle push notifications. The worker caches the versioned frontend shell and static PWA assets, but deliberately does not cache chat data, API responses, uploaded media, or live-event traffic.

Offline support means the app can open and show its normal disconnected state instead of the browser's generic offline page. It does not mean offline message history, offline search, or an outbox for composing messages while disconnected.

## Behavior

- The service worker is registered by SvelteKit in production builds.
- On install, the worker caches SvelteKit build assets, static PWA assets, and the SPA fallback shell.
- On activate, old Chatto shell caches are deleted and the new worker claims open clients.
- Known shell assets are served cache-first from the versioned cache.
- Same-origin navigations are network-first, falling back to the cached SPA shell only when the network fails.
- API, auth, webhook, uploaded asset, non-GET, and cross-origin requests are network-only.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Push dismiss payloads still close matching visible notifications on the device.

## Design Decisions

### 1. Shell-only caching

**Decision:** Cache only the app shell and static PWA assets.
**Why:** Chatto is a real-time chat app. Serving stale messages, permissions, assets, or notification state as if they were live would be worse than showing the disconnected state.
**Tradeoff:** Offline launches do not show recent rooms or messages unless the live app already has that state in memory. This keeps the data model honest.

### 2. Versioned cache names

**Decision:** Shell caches include the SvelteKit app version in their name.
**Why:** A deploy can replace hashed JavaScript and CSS chunks. Versioned cache names let the new worker populate the new shell and delete older shell caches during activation.
**Tradeoff:** A user briefly stores two shell versions during update. The cached asset set is small, so this is acceptable.

### 3. SvelteKit owns registration

**Decision:** The frontend relies on SvelteKit's production service-worker registration instead of registering manually from the push-notification setup component.
**Why:** The service worker is now useful even when Web Push is not enabled. Registration should be tied to the PWA shell, not to push settings.
**Tradeoff:** Production users get the service worker whenever the app includes one. The worker's fetch policy is conservative to make that safe.

## Related

- **FDRs:** FDR-012 (Notifications), FDR-013 (Web Push Notifications)
