# FDR-013: Web Push Notifications

**Status:** Active
**Last reviewed:** 2026-06-17

## Overview

Users can opt in to receive notifications through the browser's W3C Web Push system, so they get pinged for DMs, mentions, and replies even when the Chatto tab isn't open. Push is opt-in per device, requires operator configuration (VAPID keys), and piggybacks on the persistent notification system (see FDR-012).

## Behavior

- The browser prompts the user for notification permission when they enable push.
- If push is configured and supported, signed-in users who have not made a browser permission choice see a small top-overlay prompt offering to enable push or opt out of future prompts on that device.
- On granting permission, the browser creates a subscription using the server's VAPID public key. The subscription details (endpoint URL, keys) are sent to the server and stored.
- When a signed-in user opens Chatto and browser notification permission is already granted, Chatto refreshes the server's copy of the current browser subscription without prompting again.
- Stored subscription fields are bounded: endpoint 4,096 bytes, public key 256 bytes, auth secret 128 bytes, and user agent 512 bytes.
- A user can have multiple devices subscribed simultaneously — every device receives every push.
- Push payloads include a title, a truncated message preview (max 100 chars, broken at word boundaries), and a navigation URL.
- Clicking a push notification navigates to the relevant room, thread, or DM.
- Dismissing a notification in one place sends a "dismiss" action push to other devices, closing the system notification there too.
- Expired or invalid subscriptions (browsers report 404/410 on push delivery) are cleaned up automatically.
- Deleting the user account removes all push subscriptions.
- If the server isn't configured with VAPID keys, the push UI is hidden entirely — no opt-in prompt, no settings toggle.

## Design Decisions

### 1. Piggyback on persistent notifications

**Decision:** A push fires only when a persistent notification is created. The two share the same gating logic (mute, level, thread follow).
**Why:** Two parallel decision trees would inevitably diverge — a user who muted a room would still get pushed, or vice versa. One source of truth eliminates that bug class. See FDR-012.
**Tradeoff:** No way to push without also creating an in-app notification. Considered a feature, not a limitation: a push you can't find later in the app would be confusing.

### 2. Per-device subscriptions, identified by endpoint hash

**Decision:** Each browser subscription is stored in `RUNTIME_STATE` as its own record, identified by a hash of the push endpoint URL.
**Why:** The same user might be subscribed from a laptop and a phone, and pushing to both is the expected behavior. Hashing the endpoint URL avoids storing the raw URL as a key (which can be long and contains provider-specific structure).
**Tradeoff:** No de-duplication if a single device somehow ends up with two subscriptions. Browsers don't typically allow that, so it's a non-issue in practice.

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

## Permissions

No Chatto-side permission gates push. The OS and browser permissions are the only user-facing gates; Chatto's stored subscriptions are a refreshed delivery cache.

## Related

- **FDRs:** FDR-006 (@Mentions), FDR-012 (Notifications)
