# FDR-013: Web Push Notifications

**Status:** Active
**Last reviewed:** 2026-08-19

## Overview

Users can opt in to receive notifications through the browser's W3C Web Push system, so Alert-eligible notification activity can reach them even when the Chatto tab is not open. Push is opt-in per device, requires operator configuration (VAPID keys), and piggybacks on the persistent notification system (see FDR-012).

## Behavior

- The browser prompts the user for notification permission when they enable push.
- If push is configured and supported, signed-in users who have not made a browser permission choice see a small top-overlay prompt offering to enable push or opt out of future prompts on that device.
- On granting permission, the browser creates a subscription using the server's VAPID public key. The subscription details (endpoint URL, keys) are sent to the server and stored.
- When a signed-in user opens Chatto and browser notification permission is already granted, Chatto refreshes the server's copy of the current browser subscription without prompting again.
- A browser push endpoint is active for only the account that most recently registered it. Switching accounts in the same browser transfers delivery to the current account; stale records for the previous account are not delivered.
- In multi-server mode, native Web Push controls are shown only for the server that served the installed app. Remote servers can still update in-app notification badges and sounds while Chatto is open, but they do not offer direct browser push registration from another server's app origin.
- On iOS/iPadOS, Web Push is available only for Home Screen web apps on supported versions. Chatto treats Web Push as a notification trigger rather than authoritative app state.
- Stored subscription fields are bounded: endpoint 4,096 bytes, public key 256 bytes, auth secret 128 bytes, and user agent 512 bytes.
- Push endpoints must be absolute HTTPS URLs without user information or fragments. Delivery bypasses environment proxies, rejects redirects, and blocks private and other special-use network addresses after resolving the hostname immediately before connecting.
- A user can have up to 16 active devices subscribed simultaneously — every current device is attempted for each push. Once any device accepts an occurrence, Chatto does not retry the whole device set merely because another endpoint failed, avoiding duplicate alerts on healthy devices.
- Test notifications are limited to one attempt per account every 10 seconds across server replicas. Delivery failures expose neither provider response bodies nor low-level network errors through the public API.
- Push payloads include a mutable declarative-compatible notification envelope with a title, a message preview truncated to at most 100 Unicode characters including its ellipsis and preferring a nearby word boundary, a navigation URL, and the pending app badge count when available. The legacy root fields remain present so older Chatto service workers can display the same notification during upgrades.
- User-visible notification pushes request high-urgency delivery so mobile push services can wake sleeping devices promptly.
- Notification-alert pushes set the Web Push provider TTL to the remaining portion of the occurrence's immutable two-minute, source-time delivery window. The remaining TTL is calculated only after a bounded provider-request slot is acquired. Durable-consumer retry, backup restore, or local request contention cannot extend how long private content remains eligible at the provider.
- Clicking a push notification navigates to the relevant room, thread, or DM.
- Immediately before a regular push is sent, Chatto waits the sending replica's user and room projections through freshly captured recipient and server-wide room-event boundaries, then confirms that the occurrence is still unread and Alert-eligible, its account and membership remain active, its target message and exact reaction still exist, every prepared subscription is still owned by the recipient, and Do Not Disturb is still off. Transient projection or subscription reads fail the attempt for retry instead of being treated as absence or an empty device set. This prevents replica lag or slower asynchronous delivery from overtaking notification mutations, target removal, visibility loss, subscription rotation, or a newly enabled DND state.
- While Chatto is visible, its notification stores are authoritative for the app-icon badge. Declarative Web Push supplies the origin server's exact unread-occurrence count while the app is closed or suspended.
- Clicking or manually dismissing a native notification does not change the occurrence inside Chatto. Attention state changes only through Chatto's read and delete actions or through covered room/thread read state.
- Expired or invalid subscriptions (browsers report 404/410 on push delivery) are cleaned up automatically.
- Deleting the user account removes all push subscriptions. Cleanup is tied to
  the durable account-deletion fact, retries across crashes and partial
  failures, and rejects registration that crosses the deletion boundary. A
  renewable lease leader performs startup/periodic reconciliation without a
  fixed whole-pass deadline, using that permanent fact to erase late writes and
  repair orphaned endpoint-owner records without a second deletion marker.
- If the server isn't configured with VAPID keys, the push UI is hidden entirely — no opt-in prompt, no settings toggle.

## Design Decisions

### 1. Piggyback on persistent notifications

**Decision:** A committed notification signal is eligible to produce a push only when its source-time delivery mode is Alert. Delivery-time validation may still suppress it.
**Why:** Two parallel decision trees would inevitably diverge. One persisted policy decision and occurrence eliminate that bug class. See FDR-012.
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

### 5. Native notification state is presentation-only

**Decision:** Clicking or dismissing an OS notification does not mutate the
Chatto notification list, and in-app actions do not claim that every push service can retract
an already delivered OS notification.
**Why:** The persistent occurrence is authoritative and must not depend on
browser-specific dismissal callbacks or unordered control pushes.
**Tradeoff:** A delivered native notification can remain visible on another
device after the occurrence is triaged until the person dismisses it there.

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

**Decision:** Regular push delivery revalidates the exact unread Alert occurrence, target visibility, and active subscription immediately before sending. The visible app owns its aggregate multi-server badge; Declarative Web Push carries the origin server's exact unread-occurrence count while the app is closed.
**Why:** Occurrence materialization and push delivery are asynchronous, so a slower delivery can otherwise overtake read/delete state, target removal, or subscription rotation. Revalidation keeps the push tied to current authoritative state without persisting a separate badge record.
**Tradeoff:** The server cannot revoke a request after final validation and provider acceptance. Concurrent badge-bearing pushes remain last-delivery-wins until another push or the visible app refreshes the aggregate, and a closed-app count covers only the app's origin server.

### 11. High urgency only for user-visible pushes

**Decision:** Notification pushes request high-urgency delivery.
**Why:** Mobile operating systems may defer normal-urgency Web Push while a
device is sleeping. Alert activity is user-visible and time-sensitive, so it
should wake the device promptly. Chatto does not send separate dismissal
pushes; read and delete actions synchronize through normal app state when the client is
connected or next opens.
**Tradeoff:** Prompt delivery uses more battery than batched delivery. Restricting push to Alert occurrences keeps that cost aligned with explicit user attention policy, while an already displayed OS notification may remain until the user dismisses it.

### 12. Restricted outbound push delivery

**Decision:** Chatto accepts only absolute HTTPS push endpoints and uses a dedicated outbound client that does not use environment proxies or follow redirects. Every connection resolves the hostname once, rejects the whole result if any address is private or special-use, and connects directly to a validated address. Provider response bodies and low-level request errors are not returned to callers or written to push logs.

Existing stored endpoints receive the same checks when used. Accounts can keep at most 16 active subscriptions, delivery attempts at most those 16 endpoints, and test notifications are admitted once per 10-second shared window.

**Why:** Subscription endpoints cross an authenticated input boundary into server-side network access. Dial-time address checks cover direct internal URLs, changed DNS answers, existing records, and multi-address hostnames; refusing redirects prevents a public endpoint from handing delivery to a private destination. Generic errors remove the response-reading side channel, while the shared throttle and fan-out cap bound deliberate request amplification.

**Tradeoff:** Non-HTTPS, redirecting, private-network, or proxy-only push services are unsupported, and unusual providers cannot return diagnostic bodies through the test RPC. These endpoints are outside the browser Web Push delivery contract; operators still retain status-only push diagnostics.

## Permissions

There is no dedicated RBAC permission for Web Push. The OS/browser permission
and device subscription are the user-facing opt-in gates. Regular delivery also
requires a currently visible, unread, pending Alert occurrence within its
deadline, current notification policy and DND eligibility, an existing target,
and a subscription still owned by the recipient.

## Related

- **ADRs:** ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list)
- **FDRs:** FDR-006 (@Mentions), FDR-012 (Notifications), FDR-027 (PWA & Service Worker)
