# FDR-027: PWA & Service Worker

**Status:** Active
**Last reviewed:** 2026-10-09

## Overview

Chatto is an installable web app that requires a server connection. Service workers handle push notifications and notification clicks. The app does not keep an offline application shell or chat data on the device.

Reconnect catch-up is owned by the foreground web app. A warm reconnect keeps the normal chat layout and its retained data visible while fresh resources arrive. The worker does not cache or replay API responses or live-event traffic.

## Behavior

- Web Push uses a separate service-worker registration for each server, including the serving server. Push notifications do not require an offline application shell.
- The app does not preload the complete frontend build or provide an offline launch fallback. Normal browser HTTP caching still applies to frontend files.
- The installed app opens the origin chat route. That route opens the last room that the device remembers, or the overview.
- API, authentication, live, webhook, and uploaded-asset requests use the network.
- On each launch, the app loads chat data from the server after it verifies the session. On each page load, it also deletes the `chatto-saved-views` IndexedDB database that 0.5 beta versions created.
- On upgrade, the app removes retired offline caches and unused root registrations. It keeps a root registration while a legacy push subscription still needs it. Server-specific push registrations stay intact.
- The served web manifest uses the server name as the installed app name. Its icons, along with favicon and Apple touch icon metadata, use the uploaded server logo when one exists and fall back to bundled Chatto icons otherwise.
- Protected uploaded asset loads use direct signed asset URLs owned by the foreground app. The worker does not receive registered-server API bearer tokens, does not proxy asset requests, and does not cache protected asset bodies.
- Push notifications continue to display native OS notifications and route notification clicks into the SPA.
- The small Android notification icon uses a monochrome Chatto cat face with
  a transparent background and facial details. It is separate from the
  installed app icon and the larger colour notification icon (see FDR-013).
- Native notification dismissal is presentation-only. Chatto sends no dismissal
  control push, and dismissing an OS notification does not mutate its persistent
  occurrence. Ordinary notification updates ask visible clients to reconcile
  their authoritative list and badge.

## Design Decisions

### 1. Online application without an offline shell

**Decision:** The app loads documents and data from the server. Service workers do not cache the frontend build or provide an offline document.
**Why:** The device keeps no chat data (see ADR-107), so an offline shell cannot show rooms or messages. Preloading every feature, language, and font uses bandwidth and device storage without making the online app ready sooner.
**Tradeoff:** An installed app has no Chatto-provided launch fallback when its server is unreachable. Loaded rooms remain visible during a connection loss while the app stays open.

### 2. No chat data on the device

**Decision:** The app does not store rooms, messages, member lists, profiles, or notification state on the device. It keeps them in memory for the page session only. See ADR-107.
**Why:** A device copy needed purges at each privacy boundary, protection against stale tabs, and capture work during normal use. Its benefits were offline reading and a faster first paint on reload.
**Tradeoff:** An offline launch shows no chat content. A reload shows loading states until the server responds.

### 3. Keep push subscriptions during shell removal

**Decision:** Fresh installations create only the server-specific registrations needed for Web Push. Upgrade cleanup removes old offline caches and root registrations that have no push subscription. A subscribed root registration remains until its subscription has migrated to a server-specific registration (see FDR-013 Decision 8).
**Why:** Removing a subscribed registration would stop notifications before its replacement is ready. Repeated cleanup lets a later visit finish an interrupted upgrade without a device reset.
**Tradeoff:** A legacy root registration can remain until the user returns and its push migration completes. It uses the push-only worker and does not preload the app.

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
