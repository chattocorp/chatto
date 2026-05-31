# FDR-013: Web Push Notifications

**Status:** Active
**Last reviewed:** 2026-05-31

## Overview

Users can opt in to receive notifications through the browser's W3C Web Push system, so they get pinged for DMs, mentions, and replies even when the Chatto tab isn't open. Push is opt-in per device, requires operator configuration (VAPID keys), and piggybacks on the persistent notification system (see FDR-012).

## Behavior

- The browser prompts the user for notification permission when they enable push.
- On granting permission, the browser creates a subscription using the server's VAPID public key. The subscription details (endpoint URL, keys) are sent to the server and stored.
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

### 6. Browser subscription change detection

**Decision:** When the browser reports a subscription change (e.g., the push service rotated keys), the foreground tab is notified and re-subscribes.
**Why:** Without this, a user whose subscription expires while offline would silently stop receiving pushes. Detecting the change and re-subscribing keeps the channel alive.
**Tradeoff:** Extra plumbing. Worth it to keep the opt-in promise honest.

## Permissions

No Chatto-side permission gates push. The OS and browser permissions are the only gates.

## Related

- **FDRs:** FDR-006 (@Mentions), FDR-012 (Notifications)
