# FDR-013: Web Push Notifications

**Status:** Active
**Last reviewed:** 2026-10-03

## Overview

Users can opt in to receive notifications through the browser's W3C Web Push
system. Activity with the Push notification mode can reach them when the Chatto
tab is not open. Push is on or off per device, always for every registered
server together: it needs the browser's notification permission, and the user
can turn it off in Chatto. Push requires operator configuration (VAPID keys) and uses the
persistent notification system (see FDR-012).

## Behavior

- Chatto never asks the browser for notification permission on its own. While permission is unset and a signed-in server supports push, the chat shell shows an invitation to enable push notifications with Enable and Not now actions. Enable asks the browser from that click; the browser or operating system presents the choice. The invitation waits while a server needs sign-in, because the sign-in notice uses the same place.
- Not now hides the invitation on this device for 14 days. A dismissed browser prompt counts as Not now. If the user denies permission, Chatto does not invite again. The user can change the choice in browser or operating-system settings.
- Push is on for every registered server or for none of them. Each server's notification settings have one checkbox, Push notifications on this device. It is checked while permission is granted and the user did not turn push off. Its description names the browser and platform and states that the choice applies to every server on this device. Checking it asks the browser for permission from that click and registers every server. Unchecking it stores a device-wide opt-out first, so no tab registers any server after that, and then removes each server's browser subscription and server record. While push is off, the invitation does not appear. While the panel's server saves this device's subscription, the description shows that setup is in progress; after the save, the panel offers a test notification. Errors appear under the checkbox: a failed save with the technical reason from the browser or server, a failed turn-off, or a permission request that ended without a decision, because the user closed the prompt or the browser did not show it, with a pointer to the browser's site settings. A failed save or turn-off also offers a retry. After a failed save, the next focus or hourly check tries again. A failed save keeps the browser subscription, so retries only save it again. After a failed browser subscribe, automatic saves create no new browser subscriptions for any server until the page reloads or the user creates one with Enable, the checkbox, or a retry; browser subscribe calls run one at a time, so one failure stops the others. Chrome keeps a push service registration for each failed attempt and refuses new ones once a profile holds about 1,000. Instead of the checkbox, the panel explains blocked permission, missing browser support, and the iOS Home Screen requirement.
- Test notifications are limited to one attempt per account every 10 seconds across server replicas. Delivery failures expose neither provider response bodies nor low-level network errors through the public API.
- On granting permission, the browser creates one subscription for each eligible server. Each subscription uses that server's VAPID public key. Chatto sends each subscription to its server for storage.
- While browser notification permission is granted, Chatto registers every eligible server without asking again. This includes a server that becomes eligible later, for example after the user adds it or signs in to it again.
- A server keeps a subscription for 180 days after its most recent save. While a device uses Chatto, the app saves each subscription again when it starts. After that, it checks every server when the window gets focus and once an hour. It saves a subscription again when the last save in the page is a day old or failed, or the browser replaced the subscription. Subscriptions of a device that stops using Chatto expire.
- A browser push endpoint is active for only the account that most recently registered it. Switching accounts in the same browser transfers delivery to the current account; stale records for the previous account are not delivered.
- In multi-server mode, each authenticated server gets its own browser subscription under a stable, server-scoped service-worker registration. Each server uses its own VAPID key and sends directly to the browser push endpoint; no serving-server relay is involved.
- On iOS/iPadOS, Web Push is available only for Home Screen web apps on supported versions. In a browser tab on these systems, the server notification settings show Home Screen guidance. Chatto treats Web Push as a notification trigger rather than authoritative app state.
- Stored subscription fields are bounded: endpoint 4,096 bytes, public key 256 bytes, auth secret 128 bytes, user agent 512 bytes, and client host 255 bytes.
- Push endpoints must be absolute HTTPS URLs without user information or fragments. Delivery bypasses environment proxies, rejects redirects, and blocks private and other special-use network addresses after resolving the hostname immediately before connecting.
- An account can have up to 16 active subscriptions on each server. Every current subscription is attempted for pushes originating from that server. Once any endpoint accepts an occurrence, Chatto does not retry the complete device set only because another endpoint failed. This behavior prevents duplicate pushes on healthy devices.
- Push payloads include a mutable declarative-compatible notification envelope with a title, a message preview truncated to at most 100 Unicode characters including its ellipsis and preferring a nearby word boundary, and a navigation URL. They do not include a numeric app badge. The legacy root fields remain present so older Chatto service workers can display the same notification during upgrades.
- User-visible notification pushes request high-urgency delivery so mobile push services can wake sleeping devices promptly.
- Notification pushes set the Web Push provider TTL to the remaining portion of the occurrence's immutable two-minute, source-time delivery window. The remaining TTL is calculated only after a bounded provider-request slot is acquired. Durable-consumer retry, backup restore, or local request contention cannot extend how long private content remains eligible at the provider.
- Clicking a push notification navigates to the relevant room, thread, or DM.
- If the subscription's client host matches one of the server's configured exact public origins, regular and test notifications use that origin's scheme and the local `/chat/-` route. This includes custom domains in `webserver.allowed_origins`. Other client hosts use the sending server's primary hostname in the route. Wildcard origins do not identify aliases. Subscriptions without a client host keep the primary URL fallback.
- Immediately before a regular push is sent, Chatto waits the sending replica's user and room projections through freshly captured recipient and server-wide room-event boundaries. It then confirms that the occurrence is still unread and has the Push notification mode, its account and membership remain active, its target message and exact reaction still exist, every prepared subscription is still owned by the recipient, and Do Not Disturb is still off. Transient projection or subscription reads fail the attempt for retry instead of being treated as absence or an empty device set. This prevents replica lag or slower asynchronous delivery from overtaking notification mutations, target removal, visibility loss, subscription rotation, or a newly enabled DND state.
- While Chatto is visible, its notification stores control an unnumbered app-icon badge for Important unread attention across signed-in servers. When the service worker displays an Important push, it sets an unnumbered badge. After any push, it asks visible app windows to check current attention. Ambient pushes and older pushes without an attention level do not set a badge. If the browser displays a declarative push without running the worker, the badge stays unchanged; there is no numeric fallback.
- Clicking or manually dismissing a native notification does not change the occurrence inside Chatto. Attention state changes only through Chatto's read and delete actions or through covered room/thread read state.
- While Chatto is running, it asks the browser to close matching OS notifications after confirmed read or delete actions. It checks again after notification state updates, app focus, network recovery, and a regular push received by a visible app. Each check matches the sending server and recipient account.
- Cleanup preserves unread notifications. It also preserves uncertain results after failed requests or partial notification pages. Older payloads without account metadata remain visible until the user closes them. Closing an OS notification is best-effort and depends on browser support.
- Expired or invalid subscriptions (browsers report 404/410 on push delivery) are cleaned up automatically.
- Chatto never delivers to a subscription older than 180 days since its most recent save. Each read of an account's subscriptions removes that account's expired records, including inert records that another account took over. Push delivery and registration both do this read.
- Deleting the user account removes all push subscriptions. Cleanup is tied to
  the durable account-deletion fact, retries across crashes and partial
  failures, and rejects registration that crosses the deletion boundary. A
  renewable lease leader performs startup/periodic reconciliation without a
  fixed whole-pass deadline, using that permanent fact to erase late writes and
  repair orphaned endpoint-owner records without a second deletion marker.
- Browser push requires the Web Locks API and writable durable local storage so registration and cleanup can be serialized safely across tabs and registration suspension survives reloads.
- Signing out or removing a server writes a same-origin cross-tab suspension before cancelling active registration and queued refreshes. Per-server Web Locks serialize registration and cleanup across tabs, while storage events and a cross-tab cancellation signal release the lock even when registration is blocked on an unreachable RPC. Another tab therefore cannot recreate or adopt the shared service-worker subscription in the middle of cleanup. If an abort-insensitive save settles after another account resumes registration, account-independent cleanup presents the browser subscription's existing Push API auth secret plus its random per-save token and removes only the matching current owner/revision; this cleanup remains available after cookies or bearer tokens are revoked, and a later save reusing the same browser subscription cannot redirect it at another record. A durable same-origin refresh marker then makes the active account reassert its subscription, restoring ownership when stale work arrived last; the marker remains for the next startup until a covering save succeeds. Sign-out or server removal completes only after the browser subscription is confirmed absent or invalidated, or the server record is removed. Browser lookup failures are not treated as absence and retain the local session or server entry for a retry. Once browser invalidation succeeds, server-record cleanup remains best-effort. Only a newly authenticated session clears a sign-out/removal suspension. Earlier versions stored a per-server "disabled" suspension; the client turns one into the device-wide opt-out, so an earlier opt-out stays in effect.
- Chatto omits servers that do not have VAPID keys from push registration.

## Design Decisions

### 1. Piggyback on persistent notifications

**Decision:** A committed notification signal is eligible to produce a push
only when its materialization-time delivery mode is Push notification. Delivery-time
validation can still suppress it.
**Why:** Two parallel decision trees would inevitably diverge. One persisted policy decision and occurrence eliminate that bug class. See FDR-012.
**Tradeoff:** No way to push without also creating an in-app notification. Considered a feature, not a limitation: a push you can't find later in the app would be confusing.

### 2. Per-device subscriptions with exclusive endpoint ownership

**Decision:** Each browser subscription is stored in `RUNTIME_STATE` as its own record, identified by a hash of the push endpoint URL. A separate OCC-protected claim makes the exact current record active for only one account at a time.
**Why:** The same user might be subscribed from a laptop and a phone, and pushing to both is the expected behavior. A browser can also retain the same endpoint while the person signs out and into another account; exclusive ownership prevents pushes for the previous account from leaking into that shared browser. Tying the claim to the subscription revision also prevents a stale unsubscribe from releasing newly rotated credentials.
**Tradeoff:** Old non-owner records can remain stored but inert until normal unsubscribe, account cleanup, or expiry (see Decision 13). Records created by older versions have no claim and do not deliver until the browser reopens Chatto and performs its normal startup registration. If the browser cannot determine subscription state and the server record cannot be removed, Chatto keeps the account session or server entry in place so the user can retry instead of crossing the privacy boundary with delivery still active.

### 3. VAPID with self-managed keys

**Decision:** Operators provide a VAPID key pair and subject (contact URL). Without configuration, the feature is disabled.
**Why:** VAPID is the standard for Web Push. Self-managed keys mean the operator's server is the only entity that can send push notifications to its users — no third-party relay. Hiding the UI when unconfigured prevents user confusion.
**Tradeoff:** Operators have to generate keys and configure them. The setup docs cover this; it's a one-time cost.

### 4. Automatic cleanup of expired subscriptions

**Decision:** When a push delivery returns 404/410, the server removes that subscription record.
**Why:** Browsers expire subscriptions over time (uninstalled PWA, revoked permission, expired keys). Without cleanup, the subscription store would grow forever with dead entries, wasting send attempts.
**Tradeoff:** A transient 410 from a flaky push provider would prematurely delete an active subscription. The provider's contract is that 410 means "gone for good", so we trust it.

### 5. Native notification state is presentation-only

**Decision:** Clicking or dismissing an OS notification does not change the
Chatto notification list. The running app can close matching OS notifications
after a confirmed read or delete action, or after a fresh server read proves
that the occurrence is handled. It does not send silent dismissal pushes.
**Why:** The persistent occurrence is authoritative. Browser notification state
must not depend on optimistic changes, partial lists, or unordered control pushes.
**Tradeoff:** A closed or suspended app cannot guarantee immediate cleanup on
another device. An older notification outside a partial server page remains
visible unless this client has confirmed its handling. Payloads without the
server and account metadata are not eligible for automatic cleanup.

### 6. Startup subscription reconciliation

**Decision:** Browser/OS notification permission and the device-wide opt-out are the user-facing switches. One permission grant registers every eligible server. When a signed-in client starts and permission is already granted, it saves each current browser subscription to its server. After that, the client saves a subscription again when its server becomes eligible, when its account or VAPID key changes, when the browser replaced the subscription, when another tab requests it, when the last save in this page failed, or when the last save in this page is a day old. A local browser lookup detects a replaced subscription without a server request.
**Why:** Users should not repeat the same device permission choice for each server. Browsers, especially installed PWAs, can rotate or invalidate push subscriptions around updates. Refreshing the server-side delivery caches at startup is simpler and more reliable than depending on foreground delivery of subscription-change events. The daily refresh keeps subscriptions of a device in use from expiring (Decision 13) without a server write on every focus.
**Tradeoff:** A browser that rotates a subscription while Chatto is closed gets no pushes until Chatto opens again; while Chatto is open, the next focus or hourly check finds the rotation. An unavailable server must wait for a later reconciliation attempt. Each open tab refreshes independently, so several tabs can save the same subscription on the same day.

### 7. Invitation before the browser permission request

**Decision:** Chatto asks the browser for notification permission only when the user selects Enable in the invitation that the chat shell shows while permission is unset, or checks Push notifications on this device in a server's notification settings. Not now, or a dismissed browser prompt, hides the invitation on this device for 14 days. One grant enables push for every eligible server.
**Why:** Browsers stop showing a site's permission prompt for a while after the user dismisses it several times, and they use quieter prompts for sites where users rarely allow. A request during an unrelated click interrupts the user and uses up these chances. An explicit Enable also satisfies the user-gesture requirement of Firefox and Safari. Notification permission belongs to the installed app or browser origin, so users do not repeat the choice for each server.
**Tradeoff:** Push needs one extra click, and a user who selects Not now gets no pushes until the invitation returns or the user enables push in settings.

### 8. One native push registration per server

**Decision:** Every server, including the serving server, uses a registration of the same worker script under a stable narrow scope. This gives each server an independent browser subscription bound to its own VAPID key. Production registers the worker as a classic script. Earlier versions stored the serving server's subscription on the root registration; after the scoped subscription is saved, the client removes that old subscription from the browser and from the server. Sign-out and server removal also remove it. Every subscription records the URL host of the Chatto server that supplied the installed app with the subscription. If this host matches an exact configured server origin, the click route uses that origin's scheme and the local server route. Otherwise, the sending server combines the client host with its own primary hostname; production client hosts use HTTPS and loopback development hosts use HTTP.
**Why:** Push subscriptions belong to service-worker registrations, not to an origin as a single undifferentiated slot. Separate scopes let one installed PWA receive direct pushes from multiple servers without sharing private VAPID keys or routing notifications through the server that hosted the frontend. Push registration does not depend on the root registration, which installs the offline application shell after a delay and can fail independently. A classic registration also works in browsers without module service workers. A host is enough to reconstruct Chatto's conventional route while avoiding storage of an arbitrary client-provided navigation URL. Per-subscription client context also supports the same server account from PWAs hosted at different origins.
**Tradeoff:** Each server consumes one of the account's 16 stored subscription slots for each installed client origin. Reconstructing the scheme assumes HTTPS outside loopback development, so an HTTP PWA on a non-loopback host is unsupported. This 0.5 behavior requires the 0.5 client and server subscription contract; no pre-0.5 mixed-version path is provided.

### 9. Declarative-compatible payloads with service-worker notification fallback

**Decision:** Regular push notifications use a mutable Declarative Web Push JSON envelope while keeping the older Chatto root fields in the same payload. Numeric app badge fields are omitted.
**Why:** Modern browsers can display and navigate from the declarative notification if the service worker is unavailable. The installed worker remains a compatibility path for notification display and click routing, while older browsers and already-installed Chatto workers can keep using the legacy fields.
**Tradeoff:** Payloads duplicate a small amount of title, body, and navigation data so older workers can still display notifications. A browser that displays a push without running the worker cannot set the unnumbered badge from this payload.

### 10. Late delivery and badge ownership

**Decision:** Regular push delivery revalidates the exact unread occurrence
whose delivery mode is Push notification. It also revalidates target visibility
and the active subscription immediately before sending. The visible app owns
its unnumbered multi-server badge. The worker sets an unnumbered badge when it
displays an explicitly Important push, then asks visible windows to check
current Important attention. Ambient activity can still send a push when the
user selects that delivery mode, but it does not set an app badge.
**Why:** Occurrence materialization and push delivery are asynchronous, so a slower delivery can otherwise overtake read/delete state, target removal, or subscription rotation. Revalidation keeps the push tied to current authoritative state without persisting a separate badge record.
**Tradeoff:** The server cannot revoke a request after final validation and provider acceptance. A delayed push can set a flag after the notification was read. The visible app clears it when no Important attention remains. Without worker execution, the existing badge stays unchanged.

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
aligned with explicit user attention policy. An OS notification can remain
visible until the app can confirm its state and the browser accepts the close
request, or until the user dismisses it.

### 12. Restricted outbound push delivery

**Decision:** Chatto accepts only absolute HTTPS push endpoints and uses a dedicated outbound client that does not use environment proxies or follow redirects. Every connection resolves the hostname once, rejects the whole result if any address is private or special-use, and connects directly to a validated address. Provider response bodies and low-level request errors are not returned to callers or written to push logs.

Existing stored endpoints receive the same checks when used. Accounts can keep at most 16 active subscriptions, delivery attempts at most those 16 endpoints, and test notifications are admitted once per 10-second shared window.

**Why:** Subscription endpoints cross an authenticated input boundary into server-side network access. Dial-time address checks cover direct internal URLs, changed DNS answers, existing records, and multi-address hostnames; refusing redirects prevents a public endpoint from handing delivery to a private destination. Generic errors remove the response-reading side channel, while the shared throttle and fan-out cap bound deliberate request amplification.

**Tradeoff:** Non-HTTPS, redirecting, private-network, or proxy-only push services are unsupported, and unusual providers cannot return diagnostic bodies through the test RPC. These endpoints are outside the browser Web Push delivery contract; operators still retain status-only push diagnostics.

### 13. Subscription expiry

**Decision:** A subscription expires 180 days after its most recent save. Every save writes the save time into the record, so all server versions refresh it. Readers enforce expiry: the check immediately before a push rejects an expired record, and each read of an account's subscriptions removes that account's expired records. The removal releases the endpoint owner claim and then deletes the exact record revision that it read. There is no background sweep and no KV TTL.
**Why:** A user can register a browser and never use it again. Its credentials must not stay stored and receive private previews forever. Push delivery and registration already read all of an account's subscriptions, so removal at that read covers every account that still receives pushes without another background process. A revision-fenced delete keeps a record that the browser saved again in the meantime.
**Tradeoff:** An account that never receives another push and never registers again keeps its expired records stored, although Chatto never delivers to them. A device that does not open Chatto for 180 days must open Chatto once to receive pushes again.

### 14. One device-wide push switch for all servers

**Decision:** Push is on for every registered server or for none of them. One checkbox in any server's notification settings turns push off and on again; the choice applies to the whole device. Chatto stores the opt-out in local storage, and every registration checks it. There is no per-server push switch.
**Why:** Users must always be able to opt out and to opt back in without browser settings. A per-server switch would make the push state of a device hard to understand, and browser permission is already per origin, not per server. Per-room and per-thread notification modes still control what each server pushes.
**Tradeoff:** A user who wants pushes from only some servers must use the notification modes of the other servers instead of a device switch.

## Permissions

There is no dedicated RBAC permission for Web Push. The OS/browser permission,
the device-wide opt-out, and the device subscription are the user-facing opt-in
gates. Regular delivery also
requires a currently visible, unread, pending occurrence whose delivery mode is
Push notification. The occurrence must be within its deadline and have an
existing target. Current notification policy and DND state must permit delivery,
and the recipient must still own the subscription.

## Related

- **ADRs:** ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list)
- **FDRs:** FDR-006 (@Mentions), FDR-012 (Notifications), FDR-027 (PWA & Service Worker)
