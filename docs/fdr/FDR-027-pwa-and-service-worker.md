# FDR-027: PWA & Service Worker

**Status:** Active
**Last reviewed:** 2026-07-30

## Overview

Chatto ships a service worker so the installed web app can handle push notifications and notification clicks. The worker does not intercept network requests or cache frontend resources, chat data, API responses, live-event traffic, or protected uploaded asset bodies. Content-hashed frontend build resources instead use the browser's normal HTTP cache.

Offline launches are not supported. The PWA expects a network connection for normal use.

Reconnect catch-up is owned by the foreground web app, not the service worker. When a controlled PWA tab wakes or reconnects, server-scoped stores refetch projected ConnectRPC state and the room UI refetches the currently viewed room/thread window. The worker must not cache or replay messages, API responses, or live-event traffic.

## Behavior

- The service worker is registered by SvelteKit in production builds.
- A new worker activates promptly so an older request-intercepting worker does not remain attached to long-lived Chatto tabs.
- The worker does not intercept frontend, navigation, API, authentication, live, webhook, or uploaded-asset requests.
- Content-hashed JavaScript, CSS, and bundled font resources use normal immutable HTTP caching. Other frontend resources follow their server-provided cache policy.
- On activation, the worker removes Cache Storage left by earlier Chatto workers.
- The served web manifest uses the server name as the installed app name. Its icons, along with favicon and Apple touch icon metadata, use the uploaded server logo when one exists and fall back to bundled Chatto icons otherwise.
- Protected uploaded asset loads use direct signed asset URLs owned by the foreground app. The worker does not receive registered-server API bearer tokens, does not proxy asset requests, and does not cache protected asset bodies.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Push dismiss payloads still close matching visible notifications on the device.

## Design Decisions

### 1. No service-worker request interception

**Decision:** The service worker handles push-related events but does not handle fetches or provide an offline shell.
**Why:** Chatto is a real-time application that requires the network for useful state. Request interception adds cache policy and worker-lifecycle complexity without making the application meaningfully usable offline.
**Tradeoff:** The browser cannot launch Chatto offline through a worker-provided shell. A network error is presented directly instead of rendering an app shell whose data requests cannot succeed.

### 2. HTTP caching for frontend build resources

**Decision:** Content-hashed frontend JavaScript, CSS, and bundled fonts use immutable HTTP caching rather than being copied into Cache Storage. This policy applies only to public frontend build resources, not to uploaded assets.
**Why:** A content hash gives each build resource a new URL when its bytes change, so normal browser caching can reuse it safely without duplicating the response in a worker-managed cache.
**Tradeoff:** Cache retention is left to the browser, and non-hashed frontend resources are only reused according to their server-provided cache headers.

### 3. SvelteKit owns registration

**Decision:** The frontend relies on SvelteKit's production service-worker registration instead of registering manually from the push-notification setup component.
**Why:** Registration and worker updates belong to the installed PWA lifecycle, while push setup can independently request a subscription when the user enables notifications.
**Tradeoff:** Production users get the service worker even when they do not enable Web Push, though the dormant worker does not intercept their requests.

### 4. Protected assets bypass the worker

**Decision:** Protected uploaded assets are loaded through direct signed asset URLs and refreshed by foreground components when they approach expiry or fail to load. The service worker does not intercept, proxy, or cache those requests.
**Why:** The asset tickets and `AssetService` refresh flow are the actual reliability and authorization mechanism. Keeping asset routing out of the worker removes hidden worker/client state and keeps the service worker focused on push notifications and notification clicks.
**Tradeoff:** Ticketed asset URLs are visible in normal page markup. Their exposure is bounded by the ticket expiry and by the server's room-membership check on every fetch.

### 5. Install metadata follows server branding

**Decision:** The HTTP frontend server generates the web manifest from the bundled manifest, uses the current server name for the installed app name, and swaps in transformed server-logo URLs for install icons when a logo is configured. Stable favicon and Apple touch icon endpoints redirect to purpose-sized transforms of the current server logo, or to the bundled Chatto icons when no logo is configured.
**Why:** Self-hosted servers should install with their own visible identity without requiring a custom frontend build.
**Tradeoff:** Browsers decide when to refresh installed PWA metadata and may cache it aggressively, so existing installs or tabs may keep the previous name or icon until the browser revalidates the metadata or the user reinstalls the app.

## Related

- **ADRs:** ADR-043 (client-shell internationalization), ADR-047 (direct ticketed asset URLs)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications)
