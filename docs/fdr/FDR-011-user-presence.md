# FDR-011: User Presence

**Status:** Active
**Last reviewed:** 2026-08-30

## Overview

Every user has a presence status visible to others as a colored dot on their avatar: **Online**, **Away**, **Do Not Disturb**, or **Offline**. Presence is server-wide — a user has one status per Chatto server, not per space or room.

## Behavior

- Current clients refresh their own presence through `MyAccountService.UpdatePresence` on the ConnectRPC API.
- The client starts in Online mode unless the user previously chose another mode. Users can choose Online, Away, Do Not Disturb, or "Look offline".
- The client does not use input activity or tab visibility to change the selected mode. It does not set Away automatically.
- Users can set Do Not Disturb for their current live server presence. While DND is active, new notifications are still recorded for that user, but notification sounds and web push are suppressed (see FDR-012). Presence state is not persisted as server-side user/account state.
- Explicit Away and Do Not Disturb are marked as manually selected in the live presence record. Updates that are not manually selected do not overwrite that manual state; an explicit Online selection clears it.
- Users can choose "Look offline" locally. The client does not report an Offline status; it stops reporting presence so the existing presence record expires normally while messages and other realtime updates continue working.
- Disconnecting (closing the tab, network drop) does not send an active Offline signal. After 60 seconds without a heartbeat refresh, the presence entry expires and the user appears Offline.
- The presence dot updates across the UI as other users' statuses change, in real time.
- Room member sidebars update presence dots immediately but wait for a short quiet period before moving other users between the Online and Offline groups. Membership changes and the current user's own group movement remain immediate.
- If a live connection falls behind presence updates, it reconnects and recovers current presence instead of remaining silently stale.

## Design Decisions

### 1. Server-wide, not per-space

**Decision:** A user has one presence status across all spaces/rooms in a server.
**Why:** Anything else is confusing — "I'm online in #design but away in #engineering" doesn't match how presence works in any other chat tool. Per-server matches the user's actual session.
**Tradeoff:** Users cannot selectively appear online for some rooms. They can configure per-cause notification delivery for a room (see FDR-012), but not per-room presence.

### 2. Offline is inferred, not stored

**Decision:** Offline is the absence of a live presence record, not a stored state. Current clients refresh the user's presence entry every 30 seconds through ConnectRPC; if all clients disconnect or choose "Look offline", the entry expires after 60 seconds via NATS KV TTL.
**Why:** A disconnecting client may not get the chance to send a clean "I'm offline" message (browser crash, network drop). Relying on TTL expiry handles all the failure modes uniformly.
**Tradeoff:** Up to a one-minute delay between "user closed the tab" and "the dot turns gray". This is the standard behavior in most chat apps and matches user expectations.

### 3. User-level live status with heartbeat-driven deduplication

**Decision:** Presence is stored in `MEMORY_CACHE` as `presence.{userId}`. A per-process PresenceHub watches these keys and emits live events only when the user-level status changes. Current clients write `ONLINE`, `AWAY`, or `DO_NOT_DISTURB` through `MyAccountService.UpdatePresence`; `OFFLINE` is not an accepted update value. The live record carries whether the status was manually selected so reports that are not manually selected cannot clear explicit Away/DND.
**Why:** Presence is a current-state hint, not durable account history, but a non-manual report must not replace an explicit availability choice. Closing a tab does not actively write Offline, so another open tab can keep presence alive after the manual TTL expires.
**Tradeoff:** "Look offline" remains client-local: another active browser/device can still keep the user visible because the invisible client deliberately does not tell the server about that privacy choice.

### 4. Presence mode changes require a user choice

**Decision:** The client does not change the selected presence mode because of input inactivity or tab visibility. Online, Away, Do Not Disturb, and "Look offline" are selectable modes. Offline remains an inferred server state.
**Why:** Input activity and tab visibility are not reliable measures of availability. They can expose whether a user interacts with or views Chatto, and they can show a status that the user did not choose. Explicit mode changes keep this information under the user's control.
**Tradeoff:** A client can continue to show Online while its user is away. The status changes only when the user selects another mode or all clients stop refreshing presence and the live record expires.

### 5. DND is live user state

**Decision:** Do Not Disturb is a live presence status for the user, not durable account state. It expires with presence and is not backed up or replayed from EVT. While present, it suppresses notification sounds and Web Push at delivery time without dropping, downgrading, or rewriting the materialized delivery mode of underlying notification occurrences. Durable custom statuses live separately as user profile metadata (FDR-022).
**Why:** Presence controls notification routing and "right now" UI hints. Persisting it as domain/account history would overstate its meaning, while custom statuses communicate user-authored profile context without changing availability.
**Tradeoff:** The UI has two adjacent concepts: live presence dot and durable custom status. They deliberately answer different questions.

### 6. Invisible mode is client-local privacy behavior

**Decision:** "Look offline" is not a server status. The client stops refreshing presence while keeping its realtime event streams active. The server and other users only see the existing presence record expire.
**Why:** Reporting an explicit invisible/offline status would make the server aware of the user's privacy choice and could leak it as presence state. Keeping realtime delivery independent from presence lets the app remain fully functional without reporting the user's availability choice.
**Tradeoff:** The server can still observe ordinary authenticated activity, including API requests and an active realtime connection. "Look offline" controls the presence shown to other users; it is not an anonymity mode. Another active browser or device can also keep the user visibly present.

### 7. Per-server tracking, with frontend coordination across servers

**Decision:** Each connected Chatto server tracks its own presence. The frontend reports the chosen explicit status to all connected servers in parallel.
**Why:** Servers are independent and shouldn't have to coordinate among themselves — that would require cross-server discovery and trust. The client is already connected to all of them and can coordinate cheaply. See ADR-025.
**Tradeoff:** A user signed in from two different devices to the same server may have competing presence writers; the latest write wins until TTL expiry.

### 8. Delivery gaps force latest-value recovery

**Decision:** A connection that cannot keep up with presence transitions is closed and reconnects rather than silently dropping transitions while remaining live.
**Why:** Presence is latest-value state. Every realtime subscription includes a complete `presences_replace` reconciliation before `caught_up`, so reconnect repairs a missed transition through the same projection stream without a separate user read. Keeping an incomplete stream open would leave a presence dot stale indefinitely. See ADR-049 and ADR-051.
**Tradeoff:** A sufficiently large presence burst can reconnect a slow client, but only that lagging connection is affected and normal reconnect catch-up already handles the gap.

### 9. Presence display is immediate while member-list grouping settles

**Decision:** Presence indicators show the latest status immediately. Room member sidebars debounce presence-driven movement between their Online and Offline groups, while membership changes and the current user's own movement bypass that delay.
**Why:** Presence is useful as a current status signal, but repeatedly moving rows during a burst makes a busy member list difficult to scan and causes avoidable repeated work. Delaying only the grouping preserves freshness without making membership or the user's own action feel stale.
**Tradeoff:** Another user's row can briefly remain in its previous group while its presence dot already shows the new status. Continuous churn postpones regrouping until the updates settle.

## Permissions

Presence status is public. Any authenticated user can see any other authenticated user's presence.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-025 (multi-instance client architecture), ADR-049 (process-wide realtime event hub), ADR-051 (server-scoped resumable client projection)
- **FDRs:** FDR-012 (Notifications), FDR-022 (User Profile)
