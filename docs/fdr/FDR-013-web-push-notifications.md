# FDR-013: Web Push Notifications

**Status:** Active
**Last reviewed:** 2026-07-20

## Overview

Users can opt in to receive notifications through the browser's W3C Web Push system, so they get pinged for DMs, mentions, and replies even when the Chatto tab isn't open. Push is opt-in per device, requires operator configuration (VAPID keys), and piggybacks on the persistent notification system (see FDR-012).

## Behavior

- The browser prompts the user for notification permission when they enable push.
- If push is configured and supported, signed-in users who have not made a browser permission choice see a small top-overlay prompt offering to enable push or opt out of future prompts on that device.
- On granting permission, the browser creates a subscription using the server's VAPID public key. The subscription details (endpoint URL, keys) are sent to the server and stored.
- When a signed-in user opens Chatto and browser notification permission is already granted, Chatto refreshes the server's copy of the current browser subscription without prompting again.
- A browser push endpoint is active for only the account that most recently registered it. Switching accounts in the same browser transfers delivery to the current account; stale records for the previous account are not delivered.
- In multi-server mode, native Web Push controls are shown only for the server that served the installed app. Remote servers can still update in-app notification badges and sounds while Chatto is open, but they do not offer direct browser push registration from another server's app origin.
- On iOS/iPadOS, Web Push is available only for Home Screen web apps on supported versions. Chatto treats Web Push as a notification trigger rather than authoritative app state.
- Stored subscription fields are bounded: endpoint 4,096 bytes, public key 256 bytes, auth secret 128 bytes, and user agent 512 bytes.
- A user can have multiple devices subscribed simultaneously — every device receives every push.
- Push payloads include a mutable declarative-compatible notification envelope with a title, a truncated message preview (max 100 chars, broken at word boundaries), a navigation URL, and the pending app badge count when available. The legacy root fields remain present so older Chatto service workers can display the same notification during upgrades.
- Clicking a push notification navigates to the relevant room, thread, or DM.
- Dismissing a notification in one place sends a "dismiss" action push to other devices, closing the system notification there too.
- Immediately before a regular push is sent, Chatto confirms that the notification is still pending and the exact prepared subscription is still active. This prevents slower asynchronous creation delivery from overtaking a dismissal or subscription rotation.
- While Chatto is visible, its pending-notification stores are authoritative for the app-icon badge: an exact DM count is numeric, while other or incompletely loaded notifications use a non-numeric flag. After a regular push, the visible app reapplies that aggregate in case the browser used the push's origin-server count. Declarative Web Push supplies a numeric origin-server notification total while the app is closed or suspended. A dismiss push updates that total through the service worker when no visible app window can own the badge, because declarative push cannot remove a notification without displaying a replacement.
- Clicking or manually dismissing a native notification does not itself dismiss the pending notification inside Chatto, so it does not clear the app-icon badge; the badge clears when the pending notification is dismissed in Chatto.
- Expired or invalid subscriptions (browsers report 404/410 on push delivery) are cleaned up automatically.
- Deleting the user account removes all push subscriptions.
- If the server isn't configured with VAPID keys, the push UI is hidden entirely — no opt-in prompt, no settings toggle.

## Design Decisions

### 1. Piggyback on persistent notifications

**Decision:** A push fires only when a persistent notification is created. The two share the same gating logic (mute, level, thread follow).
**Why:** Two parallel decision trees would inevitably diverge — a user who muted a room would still get pushed, or vice versa. One source of truth eliminates that bug class. See FDR-012.
**Tradeoff:** No way to push without also creating an in-app notification. Considered a feature, not a limitation: a push you can't find later in the app would be confusing.

### 2. Per-device subscriptions with exclusive endpoint ownership

**Decision:** Each browser subscription is stored in `RUNTIME_STATE` as its own record, identified by a hash of the push endpoint URL. A separate OCC-protected claim makes the exact current record active for only one account at a time.
**Why:** The same user might be subscribed from a laptop and a phone, and pushing to both is the expected behavior. A browser can also retain the same endpoint while the person signs out and into another account; exclusive ownership prevents pushes for the previous account from leaking into that shared browser. Tying the claim to the subscription revision also prevents a stale unsubscribe from releasing newly rotated credentials.
**Tradeoff:** Old non-owner records can remain stored but inert until normal unsubscribe or account cleanup. Records created by older versions have no claim and do not deliver until the browser reopens Chatto and performs its normal startup registration.

### 3. VAPID with self-managed keys

**Decision:** Operators provide a VAPID key pair and subject (contact URL). Without configuration, the feature is disabled.
**Why:** VAPID is the standard for Web Push. Self-managed keys mean the operator's server is the only entity that can send push notifications to its users — no third-party relay. Hiding the UI when unconfigured prevents user confusion.
**Tradeoff:** Operators have to generate keys and configure them. The setup docs cover this; it's a one-time cost.

### 4. Automatic cleanup of expired subscriptions

**Decision:** When a push delivery returns 404/410, the server removes that subscription record.
**Why:** Browsers expire subscriptions over time (uninstalled PWA, revoked permission, expired keys). Without cleanup, the subscription store would grow forever with dead entries, wasting send attempts.
**Tradeoff:** A transient 410 from a flaky push provider would prematurely delete an active subscription. The provider's contract is that 410 means "gone for good", so we trust it.

### 5. Dismissal-via-push for cross-device close

**Decision:** Dismissing a notification anywhere sends a special "dismiss" payload to the user's other devices, which use it to programmatically close the system notification.
**Why:** Otherwise a notification dismissed on the laptop would linger on the phone until the user manually swiped it away. Cross-device dismiss is what users expect from modern chat apps.
**Tradeoff:** Slightly more push traffic. Bounded by user actions, so it's small.

### 6. Startup subscription reconciliation

**Decision:** Browser/OS notification permission is the user-facing source of truth. When a signed-in client starts and permission is already granted, it idempotently saves the current browser subscription to the server.
**Why:** Browsers, especially installed PWAs, can rotate or invalidate push subscriptions around updates. Refreshing the server-side delivery cache at startup is simpler and more reliable than depending on foreground delivery of subscription-change events.
**Tradeoff:** A user who grants permission but never reopens Chatto after a browser-side subscription change will not be repaired until the next app launch. That is acceptable because opening the app is the point where Chatto can reliably observe and refresh the current browser state.

### 7. Local opt-out for the push prompt

**Decision:** The enable-push prompt is device-local and can be dismissed without changing server-side notification settings.
**Why:** Whether push is useful depends on the device. Dismissing the prompt on a desktop browser should not suppress the prompt on an iOS PWA where push may be more valuable.
**Tradeoff:** The same user may see the prompt again on another browser or device. That is intentional; each device has its own push subscription and OS permission.

### 8. Origin-bound native push registration

**Decision:** Direct browser push registration is offered only for the Chatto server that served the installed web app.
**Why:** A browser push subscription belongs to a service worker origin and is created with a single application server key. Registering arbitrary remote servers from another server's app origin would imply cross-origin routing and VAPID-key behavior that Chatto has not designed yet.
**Tradeoff:** Users connected to remote servers do not get native OS notifications for those servers through this app origin. They still get realtime in-app badges and notification sounds while Chatto is open, and remote-native push can be revisited with an explicit relay or shared-key design.

### 9. Declarative-compatible payloads with service-worker notification fallback

**Decision:** Regular push notifications use a mutable Declarative Web Push JSON envelope while keeping the older Chatto root fields in the same payload. Badge counts appear in both WebKit's current top-level location and its earlier nested location during the format transition.
**Why:** Modern browsers can display and navigate from the declarative notification if the service worker is unavailable. The installed worker remains a compatibility path for notification display and click routing, while older browsers and already-installed Chatto workers can keep using the legacy fields.
**Tradeoff:** Payloads duplicate a small amount of title/body/navigation and badge data. That is preferable to a flag-day service-worker rollout or dropping badge updates on either side of WebKit's payload-format change.

### 10. Late delivery and badge ownership

**Decision:** Regular push delivery revalidates both the pending notification and exact active subscription immediately before sending. While the app is visible, direct synchronization uses a numeric badge only for an exact aggregate DM count and a flag for other or incompletely loaded notifications. After a regular push, the worker sends visible pages a value-free refresh signal so they replay that intent. Declarative push handles regular closed or suspended-app updates with the origin server's numeric total; dismiss pushes carry the remaining origin-server total and the worker applies it whenever no visible app window owns the badge.
**Why:** Notification creation and dismissal callbacks run asynchronously, so a slower creation path can otherwise finish after dismissal and restore a stale native notification. The open page must remain authoritative for its aggregate multi-server intent, including when a browser applies a later origin-only declarative badge without changing page state. The narrow refresh signal does not duplicate badge values or ownership state, while the dismiss fallback prevents cross-device dismissals from leaving a closed installed app stale. This needs no persisted badge state.
**Tradeoff:** The server check cannot revoke a request after the final validation has already passed and the push provider has accepted it. Web Push does not provide strict cross-message ordering, so concurrent badge-bearing pushes remain last-delivery-wins until another push or the visible app refreshes the aggregate. A closed app may temporarily show an all-notification number instead of the visible app's DM-count-or-flag meaning; that count also covers only the app's origin server. Browsers without declarative or worker Badging API support restore the authoritative aggregate when the app next opens.

## Permissions

No Chatto-side permission gates push. The OS and browser permissions are the only user-facing gates; Chatto's stored subscriptions are a refreshed delivery cache.

## Related

- **ADRs:** ADR-053 (convergent notification policy and pending state)
- **FDRs:** FDR-006 (@Mentions), FDR-012 (Notifications)
