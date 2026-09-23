# FDR-027: PWA & Service Worker

**Status:** Active
**Last reviewed:** 2026-09-23

## Overview

Chatto ships a service worker for push notifications, notification clicks, and an offline application shell. The root worker caches versioned frontend build files and a public login document. IndexedDB keeps limited text data for the normal chat view during an offline launch.

Offline launches show saved rooms and messages without server actions. The view is read only and can be out of date. A device cannot learn of a remote revocation while it is offline.

Reconnect catch-up is owned by the foreground web app. A warm reconnect keeps the normal chat layout and its retained data visible while fresh resources arrive. A cold offline launch restores saved rooms and messages in the same layout. The worker does not cache or replay API responses or live-event traffic.

## Behavior

- The root service worker is registered by SvelteKit in production builds. Web Push setup registers the same script under stable narrow scopes when an installed app needs independent subscriptions for remote servers.
- The root worker caches the current version of the application shell. It serves that shell on a failed navigation and serves cached compiled frontend files. Narrow push-worker registrations do not manage the shell.
- API, authentication, live, webhook, and uploaded-asset requests use the network.
- The foreground app saves the room list and up to 50 text messages from each of 10 recently viewed rooms per server and user. Saved views expire seven days after the last successful sync. The current offline storage budget is 20 MB: at most 12 MB for the shell and 8 MB for saved text.
- The app clears affected saved content on sign-out, account switch, server removal, account deletion, and verified room access loss. App preferences include a control to clear saved chats on the device.
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

**Decision:** The root worker caches only compiled frontend files and a public login document. It uses the document as a navigation fallback when the network fails.
**Why:** The normal chat view must remain available after an offline PWA launch.
**Tradeoff:** The shell uses device storage. A browser can evict it under storage pressure.

### 2. Saved text in the normal view

**Decision:** IndexedDB stores bounded, presentation-only text data under the server and user identity. The foreground app restores it into the normal chat view for the matching identity. It never stores a realtime cursor with that data and clears saved data at explicit privacy boundaries.
**Why:** People can read saved text in the familiar chat layout while offline. The saved data does not establish current authorization.
**Tradeoff:** Automatic saving puts private text on the device. A remote revocation takes effect on this copy only after the device reconnects and verifies it.

### 3. SvelteKit owns the root registration

**Decision:** The frontend relies on SvelteKit's production service-worker registration for the root worker. Push setup reuses that registration for the serving server and registers the same worker script under stable, server-specific narrow scopes for remote-server subscriptions.
**Why:** The root worker and its updates belong to the installed PWA lifecycle. A Push API subscription is bound to one service-worker registration and one application-server key, so independent scopes let remote servers retain their own VAPID keys without changing which worker controls the application page.
**Tradeoff:** Production users get the root worker even when they do not enable Web Push, and multi-server users can have additional dormant registrations after a remote subscription is removed. Only the root worker handles shell requests.

### 4. Protected assets bypass the worker

**Decision:** Protected uploaded assets are loaded through direct signed asset URLs and refreshed by foreground components when they approach expiry or fail to load. The service worker does not intercept, proxy, or cache those requests.
**Why:** The asset tickets and `AssetService` refresh flow are the actual reliability and authorization mechanism. Keeping asset routing out of the worker removes hidden worker/client state and keeps the service worker focused on push notifications and notification clicks.
**Tradeoff:** Ticketed asset URLs are visible in normal page markup. Their exposure is bounded by the ticket expiry and by the server's room-membership check on every fetch.

### 5. Install metadata follows server branding

**Decision:** The HTTP frontend server generates the web manifest from the bundled manifest, uses the current server name for the installed app name, and swaps in transformed server-logo URLs for install icons when a logo is configured. Stable favicon and Apple touch icon endpoints redirect to purpose-sized transforms of the current server logo, or to the bundled Chatto icons when no logo is configured.
**Why:** Self-hosted servers should install with their own visible identity without requiring a custom frontend build.
**Tradeoff:** Browsers decide when to refresh installed PWA metadata and may cache it aggressively, so existing installs or tabs may keep the previous name or icon until the browser revalidates the metadata or the user reinstalls the app.

## Related

- **ADRs:** ADR-047 (direct ticketed asset URLs), ADR-065 (runtime JSON client internationalization), ADR-067 (Electron desktop packaging)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-012 (Notifications), FDR-013 (Web Push Notifications), FDR-034 (Chatto Desktop)
