# FDR-013: Web Push Notifications

**Status:** Active
**Last reviewed:** 2026-08-30

## Overview

Users can opt in to receive notifications through the browser's W3C Web Push
system. Activity with the Push notification mode can reach them when the Chatto
tab is not open. Push is opt-in for each device, requires operator configuration
(VAPID keys), and uses the persistent notification system (see FDR-012).

## Behavior

- If push is configured and supported, the Notifications pane shows an action to enable push while browser permission is unset. This action opens the browser or operating-system permission prompt.
- If the user dismisses the browser prompt, the action remains available because permission is still unset. If the user denies permission, Chatto hides the action. The user can change the choice in browser or operating-system settings.
- On granting permission, the browser creates one subscription for each eligible server. Each subscription uses that server's VAPID public key. Chatto sends each subscription to its server for storage.
- When a signed-in user opens Chatto and browser notification permission is already granted, Chatto refreshes every eligible server's subscription without prompting again. Chatto also registers a server that becomes eligible later.
- A browser push endpoint is active for only the account that most recently registered it. Switching accounts in the same browser transfers delivery to the current account; stale records for the previous account are not delivered.
- In multi-server mode, each authenticated server gets its own browser subscription under a stable, server-scoped service-worker registration. Each server uses its own VAPID key and sends directly to the browser push endpoint; no serving-server relay is involved.
- On iOS/iPadOS, Web Push is available only for Home Screen web apps on supported versions. Chatto treats Web Push as a notification trigger rather than authoritative app state.
- Stored subscription fields are bounded: endpoint 4,096 bytes, public key 256 bytes, auth secret 128 bytes, user agent 512 bytes, and client host 255 bytes.
- Push endpoints must be absolute HTTPS URLs without user information or fragments. Delivery bypasses environment proxies, rejects redirects, and blocks private and other special-use network addresses after resolving the hostname immediately before connecting.
- An account can have up to 16 active subscriptions on each server. Every current subscription is attempted for pushes originating from that server. Once any endpoint accepts an occurrence, Chatto does not retry the complete device set only because another endpoint failed. This behavior prevents duplicate pushes on healthy devices.
- Test notifications are limited to one attempt per account every 10 seconds across server replicas. Delivery failures expose neither provider response bodies nor low-level network errors through the public API.
- Push payloads include a mutable declarative-compatible notification envelope with a title, a message preview truncated to at most 100 Unicode characters including its ellipsis and preferring a nearby word boundary, a navigation URL, and the pending app badge count when available. The legacy root fields remain present so older Chatto service workers can display the same notification during upgrades.
- User-visible notification pushes request high-urgency delivery so mobile push services can wake sleeping devices promptly.
- Notification pushes set the Web Push provider TTL to the remaining portion of the occurrence's immutable two-minute, source-time delivery window. The remaining TTL is calculated only after a bounded provider-request slot is acquired. Durable-consumer retry, backup restore, or local request contention cannot extend how long private content remains eligible at the provider.
- Clicking a push notification navigates to the relevant room, thread, or DM.
- Immediately before a regular push is sent, Chatto waits the sending replica's user and room projections through freshly captured recipient and server-wide room-event boundaries. It then confirms that the occurrence is still unread and has the Push notification mode, its account and membership remain active, its target message and exact reaction still exist, every prepared subscription is still owned by the recipient, and Do Not Disturb is still off. Transient projection or subscription reads fail the attempt for retry instead of being treated as absence or an empty device set. This prevents replica lag or slower asynchronous delivery from overtaking notification mutations, target removal, visibility loss, subscription rotation, or a newly enabled DND state.
- While Chatto is visible, its notification stores are authoritative for the aggregate app-icon badge. Declarative Web Push supplies the sending server's exact unread-occurrence count while the app is closed or suspended.
- Clicking or manually dismissing a native notification does not change the occurrence inside Chatto. Attention state changes only through Chatto's read and delete actions or through covered room/thread read state.
- Expired or invalid subscriptions (browsers report 404/410 on push delivery) are cleaned up automatically.
- Deleting the user account removes all push subscriptions. Cleanup is tied to
  the durable account-deletion fact, retries across crashes and partial
  failures, and rejects registration that crosses the deletion boundary. A
  renewable lease leader performs startup/periodic reconciliation without a
  fixed whole-pass deadline, using that permanent fact to erase late writes and
  repair orphaned endpoint-owner records without a second deletion marker.
- Browser push requires the Web Locks API and writable durable local storage so registration and cleanup can be serialized safely across tabs and registration suspension survives reloads.
- Disabling push, signing out, or removing a server writes a same-origin cross-tab suspension before cancelling active registration and queued refreshes. Per-server Web Locks serialize registration and cleanup across tabs, while storage events and a cross-tab cancellation signal release the lock even when registration is blocked on an unreachable RPC. Another tab therefore cannot recreate or adopt the shared service-worker subscription in the middle of cleanup. If an abort-insensitive save settles after another account resumes registration, account-independent cleanup presents the browser subscription's existing Push API auth secret plus its random per-save token and removes only the matching current owner/revision; this cleanup remains available after cookies or bearer tokens are revoked, and a later save reusing the same browser subscription cannot redirect it at another record. A durable same-origin refresh marker then makes the active account reassert its subscription, restoring ownership when stale work arrived last; the marker remains for the next startup until a covering save succeeds. Sign-out or server removal completes only after the browser subscription is confirmed absent or invalidated, or the server record is removed. Browser lookup failures are not treated as absence and retain the local session or server entry for a retry. Once browser invalidation succeeds, server-record cleanup remains best-effort. Explicit re-enabling clears a disabled suspension, while only a newly authenticated session clears a sign-out/removal suspension.
- Chatto omits servers that do not have VAPID keys from push registration. The server notification settings hide push controls when the selected server does not support push.

## Design Decisions

### 1. Piggyback on persistent notifications

**Decision:** A committed notification signal is eligible to produce a push
only when its source-time delivery mode is Push notification. Delivery-time
validation can still suppress it.
**Why:** Two parallel decision trees would inevitably diverge. One persisted policy decision and occurrence eliminate that bug class. See FDR-012.
**Tradeoff:** No way to push without also creating an in-app notification. Considered a feature, not a limitation: a push you can't find later in the app would be confusing.

### 2. Per-device subscriptions with exclusive endpoint ownership

**Decision:** Each browser subscription is stored in `RUNTIME_STATE` as its own record, identified by a hash of the push endpoint URL. A separate OCC-protected claim makes the exact current record active for only one account at a time.
**Why:** The same user might be subscribed from a laptop and a phone, and pushing to both is the expected behavior. A browser can also retain the same endpoint while the person signs out and into another account; exclusive ownership prevents pushes for the previous account from leaking into that shared browser. Tying the claim to the subscription revision also prevents a stale unsubscribe from releasing newly rotated credentials.
**Tradeoff:** Old non-owner records can remain stored but inert until normal unsubscribe or account cleanup. Records created by older versions have no claim and do not deliver until the browser reopens Chatto and performs its normal startup registration. If the browser cannot determine subscription state and the server record cannot be removed, Chatto keeps the account session or server entry in place so the user can retry instead of crossing the privacy boundary with delivery still active.

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

**Decision:** Browser/OS notification permission is the user-facing source of truth. One permission grant registers every eligible server. When a signed-in client starts and permission is already granted, it idempotently saves each current browser subscription to its server.
**Why:** Users should not repeat the same device permission choice for each server. Browsers, especially installed PWAs, can rotate or invalidate push subscriptions around updates. Refreshing the server-side delivery caches at startup is simpler and more reliable than depending on foreground delivery of subscription-change events.
**Tradeoff:** Reconciliation can make an idempotent subscription request when eligible servers change, when the app regains focus, or when the service worker changes. An unavailable server must wait for a later reconciliation attempt.

### 7. Explicit browser permission request

**Decision:** When permission is unset, the Notifications pane provides one explicit action that requests browser permission and enables push for every eligible server.
**Why:** Browsers require notification permission requests to follow a clear user action. A visible action is reliable and gives the prompt relevant context. Notification permission still belongs to the installed app or browser origin, so users do not repeat the choice for each server.
**Tradeoff:** A user must open the Notifications pane and select the action. Chatto does not interrupt unrelated interactions with a permission prompt.

### 8. One native push registration per server

**Decision:** The serving server uses SvelteKit's root service-worker registration. Every remote server uses another registration of the same worker script under a stable narrow scope, giving that server an independent browser subscription bound to its own VAPID key. Every subscription records the URL host of the Chatto server that supplied the installed app with the subscription. The sending server combines that client host with its own hostname to reconstruct the click route; production client hosts use HTTPS and loopback development hosts use HTTP.
**Why:** Push subscriptions belong to service-worker registrations, not to an origin as a single undifferentiated slot. Separate scopes let one installed PWA receive direct pushes from multiple servers without sharing private VAPID keys or routing notifications through the server that hosted the frontend. A host is enough to reconstruct Chatto's conventional route while avoiding storage of an arbitrary client-provided navigation URL. Per-subscription client context also supports the same server account from PWAs hosted at different origins.
**Tradeoff:** Each server consumes one of the account's 16 stored subscription slots for each installed client origin. Reconstructing the scheme assumes HTTPS outside loopback development, so an HTTP PWA on a non-loopback host is unsupported. This 0.5 behavior requires the 0.5 client and server subscription contract; no pre-0.5 mixed-version path is provided.

### 9. Declarative-compatible payloads with service-worker notification fallback

**Decision:** Regular push notifications use a mutable Declarative Web Push JSON envelope while keeping the older Chatto root fields in the same payload. Badge counts appear in both WebKit's current top-level location and its earlier nested location during the format transition.
**Why:** Modern browsers can display and navigate from the declarative notification if the service worker is unavailable. The installed worker remains a compatibility path for notification display and click routing, while older browsers and already-installed Chatto workers can keep using the legacy fields.
**Tradeoff:** Payloads duplicate a small amount of title/body/navigation and badge data. That is preferable to a flag-day service-worker rollout or dropping badge updates on either side of WebKit's payload-format change.

### 10. Late delivery and badge ownership

**Decision:** Regular push delivery revalidates the exact unread occurrence
whose delivery mode is Push notification. It also revalidates target visibility
and the active subscription immediately before sending. The visible app owns
its aggregate multi-server badge;
Declarative Web Push carries the sending server's exact unread-occurrence count
while the app is closed.
**Why:** Occurrence materialization and push delivery are asynchronous, so a slower delivery can otherwise overtake read/delete state, target removal, or subscription rotation. Revalidation keeps the push tied to current authoritative state without persisting a separate badge record.
**Tradeoff:** The server cannot revoke a request after final validation and provider acceptance. Concurrent badge-bearing pushes remain last-delivery-wins until another push or the visible app refreshes the aggregate, and a closed-app count reflects only the server whose push arrived last.

### 11. High urgency only for user-visible pushes

**Decision:** Notification pushes request high-urgency delivery.
**Why:** Mobile operating systems may defer normal-urgency Web Push while a
device is sleeping. Push notification activity is user-visible and
time-sensitive, so it
should wake the device promptly. Chatto does not send separate dismissal
pushes; read and delete actions synchronize through normal app state when the client is
connected or next opens.
**Tradeoff:** Prompt delivery uses more battery than batched delivery.
Restricting push to occurrences with the Push notification mode keeps that cost
aligned with explicit user attention policy. An OS notification that is already
visible can remain until the user dismisses it.

### 12. Restricted outbound push delivery

**Decision:** Chatto accepts only absolute HTTPS push endpoints and uses a dedicated outbound client that does not use environment proxies or follow redirects. Every connection resolves the hostname once, rejects the whole result if any address is private or special-use, and connects directly to a validated address. Provider response bodies and low-level request errors are not returned to callers or written to push logs.

Existing stored endpoints receive the same checks when used. Accounts can keep at most 16 active subscriptions, delivery attempts at most those 16 endpoints, and test notifications are admitted once per 10-second shared window.

**Why:** Subscription endpoints cross an authenticated input boundary into server-side network access. Dial-time address checks cover direct internal URLs, changed DNS answers, existing records, and multi-address hostnames; refusing redirects prevents a public endpoint from handing delivery to a private destination. Generic errors remove the response-reading side channel, while the shared throttle and fan-out cap bound deliberate request amplification.

**Tradeoff:** Non-HTTPS, redirecting, private-network, or proxy-only push services are unsupported, and unusual providers cannot return diagnostic bodies through the test RPC. These endpoints are outside the browser Web Push delivery contract; operators still retain status-only push diagnostics.

## Permissions

There is no dedicated RBAC permission for Web Push. The OS/browser permission
and device subscription are the user-facing opt-in gates. Regular delivery also
requires a currently visible, unread, pending occurrence whose delivery mode is
Push notification. The occurrence must be within its deadline and have an
existing target. Current notification policy and DND state must permit delivery,
and the recipient must still own the subscription.

## Related

- **ADRs:** ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list)
- **FDRs:** FDR-006 (@Mentions), FDR-012 (Notifications), FDR-027 (PWA & Service Worker)
