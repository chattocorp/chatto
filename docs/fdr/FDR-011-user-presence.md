# FDR-011: User Presence

**Status:** Active
**Last reviewed:** 2026-06-01

## Overview

Every user has a presence status visible to others as a colored dot on their avatar: **Online**, **Away**, **Do Not Disturb**, or **Offline**. Presence is server-wide — a user has one status per Chatto server, not per space or room.

## Behavior

- When a user connects to a server (subscribes to the main event stream), the server sets that user's presence to Online and keeps the TTL refreshed.
- After 5 minutes without keyboard/mouse/touch input, the client transitions to Away.
- If the browser tab is hidden for 10 seconds, the client also transitions to Away (debounced to avoid flashing during quick tab switches).
- Any interaction returns the user to Online.
- Users can set Do Not Disturb for their current live server presence. Presence state is not persisted as a user preference.
- Disconnecting (closing the tab, network drop) does not send an active Offline signal. After 60 seconds without a heartbeat refresh, the presence entry expires and the user appears Offline.
- The presence dot updates across the UI as other users' statuses change, in real time.

## Design Decisions

### 1. Server-wide, not per-space

**Decision:** A user has one presence status across all spaces/rooms in a server.
**Why:** Anything else is confusing — "I'm online in #design but away in #engineering" doesn't match how presence works in any other chat tool. Per-server matches the user's actual session.
**Tradeoff:** Users can't selectively appear online for some rooms. They can mute rooms for notification purposes (see FDR-012) but not for presence.

### 2. Offline is inferred, not stored

**Decision:** Offline is the absence of a live presence record, not a stored state. The server refreshes the user's presence entry every 30 seconds; if all clients disconnect, the entry expires after 60 seconds via NATS KV TTL.
**Why:** A disconnecting client may not get the chance to send a clean "I'm offline" message (browser crash, network drop). Relying on TTL expiry handles all the failure modes uniformly.
**Tradeoff:** Up to a one-minute delay between "user closed the tab" and "the dot turns gray". This is the standard behavior in most chat apps and matches user expectations.

### 3. User-level live status with heartbeat-driven deduplication

**Decision:** Presence is stored in `MEMORY_CACHE` as `presence.{userId}`. A per-process PresenceHub watches these keys and emits live events only when the user-level status changes.
**Why:** Presence is a current-state hint, not per-tab/device domain state. Competing tabs/devices simply write the user's current live status; closing a tab does not actively write Offline, so another open tab can keep the TTL alive.
**Tradeoff:** The frontend has to clear its presence cache on reconnect, because it can't rely on incremental updates if it dropped a status-change event mid-flight.

### 4. Auto-away has two triggers (idle + tab hidden)

**Decision:** Two independent triggers transition to Away: 5 minutes of input inactivity, OR 10 seconds of tab hidden.
**Why:** Idle-only would miss the very common "switched tabs" case. Tab-hidden-only would mark people as away the moment they alt-tab to look at something. Combining both covers the realistic away cases.
**Tradeoff:** Some false positives — a user actively listening in another window is technically "away" by our model. Acceptable for the use case.

### 5. DND is live user state

**Decision:** Do Not Disturb is a live presence status for the user, not durable account state. It expires with presence and is not backed up or replayed from EVT.
**Why:** Presence controls notification routing and "right now" UI hints. Persisting it as domain/account history would overstate its meaning.
**Tradeoff:** A future durable "manual status" feature would need separate product semantics rather than reusing transient presence records.

### 6. Per-server tracking, with frontend coordination across servers

**Decision:** Each connected Chatto server tracks its own presence. The frontend's auto-away detector broadcasts the new status to all connected servers in parallel.
**Why:** Servers are independent and shouldn't have to coordinate among themselves — that would require cross-server discovery and trust. The client is already connected to all of them and can coordinate cheaply. See ADR-025.
**Tradeoff:** A user signed in from two different devices to the same server may have competing presence writers; the latest write wins until TTL expiry.

## Permissions

Presence status is public. Any authenticated user can see any other authenticated user's presence.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-012 (Notifications)
