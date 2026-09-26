# FDR-027: PWA & Service Worker

**Status:** Active
**Last reviewed:** 2026-09-26

## Overview

Chatto ships a service worker for push notifications, notification clicks, and an offline application shell. The root worker caches versioned frontend build files and a public login document. The device keeps no chat data. An offline launch loads the shell, but it cannot show rooms or messages.

Reconnect catch-up is owned by the foreground web app. A warm reconnect keeps the normal chat layout and its retained data visible while fresh resources arrive. The worker does not cache or replay API responses or live-event traffic.

## Behavior

- The foreground app registers the root service worker shortly after startup in production builds. Web Push setup registers the same script under stable narrow scopes when an installed app needs independent subscriptions for remote servers.
- The root worker caches the current version of the application shell. On a later app launch, it serves the complete cached shell immediately. It uses the network when the cache is absent. Narrow push-worker registrations do not manage the shell.
- The installed app opens the origin chat route. That route opens the last room that the device remembers, or the overview.
- API, authentication, live, webhook, and uploaded-asset requests use the network.
- The shell cache uses at most 12 MB.
- On each launch, the app loads chat data from the server after it verifies the session. On each page load, it also deletes the `chatto-saved-views` IndexedDB database that 0.5 beta versions created.
- On activation, the root worker removes older shell caches after the new shell is installed.
- The served web manifest uses the server name as the installed app name. Its icons, along with favicon and Apple touch icon metadata, use the uploaded server logo when one exists and fall back to bundled Chatto icons otherwise.
- Protected uploaded asset loads use direct signed asset URLs owned by the foreground app. The worker does not receive registered-server API bearer tokens, does not proxy asset requests, and does not cache protected asset bodies.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Native notification dismissal is presentation-only. Chatto sends no dismissal
  control push, and dismissing an OS notification does not mutate its persistent
  occurrence. Ordinary notification updates ask visible clients to reconcile
  their authoritative list and badge.

## Design Decisions

### 1. Versioned offline shell

**Decision:** The root worker caches only compiled frontend files and a public login document. It serves that version-matched document first for app routes on a later launch. It uses the network when the cache is absent.
**Why:** The app shell must open quickly on a repeat launch, including when the server is slow or offline.
**Tradeoff:** A repeat launch can open the previous installed frontend version until the worker update completes. The shell uses device storage, which the browser can evict under storage pressure.

### 2. No chat data on the device

**Decision:** The app does not store rooms, messages, member lists, profiles, or notification state on the device. It keeps them in memory for the page session only. See ADR-107.
**Why:** A device copy needed purges at each privacy boundary, protection against stale tabs, and capture work during normal use. Its benefit was offline reading and a faster first paint on reload.
**Tradeoff:** An offline launch shows no chat content. A reload shows loading states until the server responds.

### 3. Foreground app owns the root registration

**Decision:** The foreground app registers the root worker after a short startup delay. Push setup reuses that registration for the serving server and registers the same worker script under stable, server-specific narrow scopes for remote-server subscriptions.
**Why:** Preloading the complete shell during the first navigation delays the page. A Push API subscription is bound to one service-worker registration and one application-server key, so independent scopes let remote servers retain their own VAPID keys without changing which worker controls the application page.
**Tradeoff:** The offline shell becomes available only after the startup delay and cache installation finish. Production users get the root worker even when they do not enable Web Push, and multi-server users can have additional dormant registrations after a remote subscription is removed. Only the root worker handles shell requests.

### 4. Protected assets bypass the worker

**Decision:** Protected uploaded assets are loaded through direct signed asset URLs and refreshed by foreground components when they approach expiry or fail to load. The service worker does not intercept, proxy, or cache those requests.
**Why:** The asset tickets and `AssetService` refresh flow are the actual reliability and authorization mechanism. Keeping asset routing out of the worker removes hidden worker/client state and keeps the service worker focused on push notifications and notification clicks.
**Tradeoff:** Ticketed asset URLs are visible in normal page markup. Their exposure is bounded by the ticket expiry and by the server's room-membership check on every fetch.

### 5. Install metadata follows server branding

**Decision:** The HTTP frontend server generates the web manifest from the bundled manifest, uses the current server name for the installed app name, and swaps in transformed server-logo URLs for install icons when a logo is configured. Stable favicon and Apple touch icon endpoints redirect to purpose-sized transforms of the current server logo, or to the bundled Chatto icons when no logo is configured.
**Why:** Self-hosted servers should install with their own visible identity without requiring a custom frontend build.
**Tradeoff:** Browsers decide when to refresh installed PWA metadata and may cache it aggressively, so existing installs or tabs may keep the previous name or icon until the browser revalidates the metadata or the user reinstalls the app.

## Related

- **ADRs:** ADR-047 (direct ticketed asset URLs), ADR-065 (runtime JSON client internationalization), ADR-067 (Electron desktop packaging), ADR-103 (cached-first client startup, shell decision only), [ADR-107](../adr/ADR-107-keep-chat-data-out-of-device-storage.md) (no chat data in device storage)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications), FDR-034 (Chatto Desktop)
