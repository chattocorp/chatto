# FDR-027: PWA & Service Worker

**Status:** Active
**Last reviewed:** 2026-10-04

## Overview

Chatto ships a service worker for push notifications, notification clicks, and an offline application shell. The root worker caches versioned frontend build files and a public login document. The device keeps no chat data. An offline launch loads the shell, but it cannot show rooms or messages.

Reconnect catch-up is owned by the foreground web app. A warm reconnect keeps the normal chat layout and its retained data visible while fresh resources arrive. The worker does not cache or replay API responses or live-event traffic.

## Behavior

- The foreground app registers the root service worker shortly after startup in production builds. Web Push setup registers the same script under a stable narrow scope for each server, including the serving server. Production registers these push workers as classic scripts.
- The root worker caches the current version of the application shell. App navigations load the document from the network first. The worker serves the cached shell document when the network request fails or the server returns a server error. Narrow push-worker registrations do not manage the shell.
- The installed app opens the origin chat route. That route opens the last room that the device remembers, or the overview.
- API, authentication, live, webhook, and uploaded-asset requests use the network.
- The shell cache uses at most 12 MB.
- On each launch, the app loads chat data from the server after it verifies the session. On each page load, it also deletes the `chatto-saved-views` IndexedDB database that 0.5 beta versions created.
- Each build installs into its own shell cache, also when local builds repeat the version name. On activation, the root worker removes older shell caches after the new shell is installed.
- The served web manifest uses the server name as the installed app name. Its icons, along with favicon and Apple touch icon metadata, use the uploaded server logo when one exists and fall back to bundled Chatto icons otherwise.
- Protected uploaded asset loads use direct signed asset URLs owned by the foreground app. The worker does not receive registered-server API bearer tokens, does not proxy asset requests, and does not cache protected asset bodies.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- Native notification dismissal is presentation-only. Chatto sends no dismissal
  control push, and dismissing an OS notification does not mutate its persistent
  occurrence. Ordinary notification updates ask visible clients to reconcile
  their authoritative list and badge.

## Design Decisions

### 1. Versioned offline shell

**Decision:** The root worker caches only compiled frontend files and a public login document. App navigations load the document from the network first. The worker serves the cached document when the network request fails or the server returns a server error.
**Why:** The app keeps no chat data on the device (see ADR-107), so a cached document cannot show content sooner than the server. A network document makes the first reload after a deploy load the new frontend version. The cached document still opens the app when the server is unreachable.
**Tradeoff:** An online launch waits for the server's document. The shell uses device storage, which the browser can evict under storage pressure.

### 2. No chat data on the device

**Decision:** The app does not store rooms, messages, member lists, profiles, or notification state on the device. It keeps them in memory for the page session only. See ADR-107.
**Why:** A device copy needed purges at each privacy boundary, protection against stale tabs, and capture work during normal use. Its benefits were offline reading and a faster first paint on reload.
**Tradeoff:** An offline launch shows no chat content. A reload shows loading states until the server responds.

### 3. Foreground app owns the root registration

**Decision:** The foreground app registers the root worker after a short startup delay. The root registration serves only the offline shell. Push setup registers the same worker script under a stable, server-specific narrow scope for every server, including the serving server (see FDR-013 Decision 8).
**Why:** Preloading the complete shell during the first navigation delays the page. A Push API subscription is bound to one service-worker registration and one application-server key, so independent scopes let each server retain its own VAPID key without changing which worker controls the application page. Push registration does not wait for the delayed root registration or depend on a successful shell installation.
**Tradeoff:** The offline shell becomes available only after the startup delay and cache installation finish. Production users get the root worker even when they do not enable Web Push, and users can have additional dormant push registrations after a subscription is removed. Only the root worker handles shell requests.

### 4. Protected assets bypass the worker

**Decision:** Protected uploaded assets are loaded through direct signed asset URLs and refreshed by foreground components when they approach expiry or fail to load. The service worker does not intercept, proxy, or cache those requests.
**Why:** The asset tickets and `AssetService` refresh flow are the actual reliability and authorization mechanism. Keeping asset routing out of the worker removes hidden worker/client state and keeps the service worker focused on push notifications and notification clicks.
**Tradeoff:** Ticketed asset URLs are visible in normal page markup. Their exposure is bounded by the ticket expiry and by the server's room-membership check on every fetch.

### 5. Install metadata follows server branding

**Decision:** The HTTP frontend server generates the web manifest from the bundled manifest, uses the current server name for the installed app name, and swaps in transformed server-logo URLs for install icons when a logo is configured. Stable favicon and Apple touch icon endpoints redirect to purpose-sized transforms of the current server logo, or to the bundled Chatto icons when no logo is configured.
**Why:** Self-hosted servers should install with their own visible identity without requiring a custom frontend build.
**Tradeoff:** Browsers decide when to refresh installed PWA metadata and may cache it aggressively, so existing installs or tabs may keep the previous name or icon until the browser revalidates the metadata or the user reinstalls the app.

### 6. Notification clicks focus an open window and send it the target

**Decision:** A notification click focuses one open application window and then sends it the target URL. The page routes in place with client-side navigation. The worker prefers a focused window, then a visible window, then a hidden window. It ignores same-origin windows that do not run the application, such as an opened attachment under `/assets`. When no application window is open, or focus fails, the worker opens the target in a new window. The worker does not wait for a reply from the page, and it does not use `WindowClient.navigate()`.
**Why:** This is the usual pattern for notification clicks. The push worker has a narrow scope, so it never controls an application window, and browsers reject `navigate()` from a worker that does not control the window. Browsers allow window actions only briefly after a click, and Chromium allows only one action: `focus()` or `openWindow()`. An earlier design focused each window and then waited up to 750 ms for a reply before it used `navigate()` or `openWindow()`. In Chromium, the first focus used the click, so the fallback could not open a window. In Firefox, the fallback ran near the one-second click limit, and a late reply opened a second window. A frozen background page on a mobile device cannot reply, but it receives the message when focus resumes it.
**Tradeoff:** A window that has not yet registered its click listener, such as a page that is still loading, gets focus but does not route. `Client.url` gives the URL that loaded a window, not its current route. For this reason, the worker cannot identify OAuth windows, and a focused OAuth window can receive the click. Pages still reply when the message has a reply port, because workers from earlier releases wait for the reply.

## Related

- **ADRs:** ADR-047 (direct ticketed asset URLs), ADR-065 (runtime JSON client internationalization), ADR-067 (Electron desktop packaging), ADR-103 (cached-first client startup, superseded), [ADR-107](../adr/ADR-107-keep-chat-data-out-of-device-storage.md) (no chat data in device storage)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications), FDR-034 (Chatto Desktop)
