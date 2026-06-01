# FDR-011: User Presence

**Status:** Active
**Last reviewed:** 2026-06-01

## Overview

Every user has a presence status visible to others as a colored dot on their avatar: **Online**, **Away**, **Do Not Disturb**, or **Offline**. Presence is server-wide — a user has one status per Chatto server, not per space or room.

## Behavior

- When a user connects to a server (subscribes to the main event stream), the client supplies a per-tab/per-device presence session ID and that session is set to Online.
- After 5 minutes without keyboard/mouse/touch input, the client transitions to Away.
- If the browser tab is hidden for 10 seconds, the client also transitions to Away (debounced to avoid flashing during quick tab switches).
- Any interaction returns the user to Online.
- Users can set Do Not Disturb for the current live session. Presence state is not persisted as a user preference.
- Disconnecting (closing the tab, network drop) leaves that session entry to expire. After 60 seconds without a heartbeat refresh, the session is gone; the user appears Offline only when no other live session remains.
- The presence dot updates across the UI as other users' statuses change, in real time.

## Design Decisions

### 1. Server-wide, not per-space

**Decision:** A user has one presence status across all spaces/rooms in a server.
**Why:** Anything else is confusing — "I'm online in #design but away in #engineering" doesn't match how presence works in any other chat tool. Per-server matches the user's actual session.
**Tradeoff:** Users can't selectively appear online for some rooms. They can mute rooms for notification purposes (see FDR-012) but not for presence.

### 2. Offline is inferred, not stored

**Decision:** Offline is the absence of any live presence session, not a stored state. The server refreshes the session entry every 30 seconds; if the client disconnects, the entry expires after 60 seconds via NATS KV TTL.
**Why:** A disconnecting client may not get the chance to send a clean "I'm offline" message (browser crash, network drop). Relying on TTL expiry handles all the failure modes uniformly.
**Tradeoff:** Up to a one-minute delay between "user closed the tab" and "the dot turns gray". This is the standard behavior in most chat apps and matches user expectations.

### 3. Per-session aggregation with heartbeat-driven deduplication

**Decision:** Presence is stored in `MEMORY_CACHE` as `presence_session.{userId}.{sessionId}`. A per-process PresenceHub watches these keys, derives one effective status per user, and emits live events only when the derived status changes. Effective precedence is Do Not Disturb, then Online, then Away, then Offline.
**Why:** Multi-tab and multi-device users should stay online while any session is online. Broadcasting a "I'm still Online" event every 30 seconds to every connected client would generate enormous useless traffic. Only derived user-level changes carry value.
**Tradeoff:** The frontend has to clear its presence cache on reconnect, because it can't rely on incremental updates if it dropped a status-change event mid-flight.

### 4. Auto-away has two triggers (idle + tab hidden)

**Decision:** Two independent triggers transition to Away: 5 minutes of input inactivity, OR 10 seconds of tab hidden.
**Why:** Idle-only would miss the very common "switched tabs" case. Tab-hidden-only would mark people as away the moment they alt-tab to look at something. Combining both covers the realistic away cases.
**Tradeoff:** Some false positives — a user actively listening in another window is technically "away" by our model. Acceptable for the use case.

### 5. DND is live session state

**Decision:** Do Not Disturb is a live presence status on a session, not durable account state. It expires with the session and is not backed up or replayed from EVT.
**Why:** Presence controls notification routing and "right now" UI hints. Persisting it as domain/account history would overstate its meaning.
**Tradeoff:** A future durable "manual status" feature would need separate product semantics rather than reusing transient presence records.

### 6. Per-server tracking, with frontend coordination across servers

**Decision:** Each connected Chatto server tracks its own presence. The frontend's auto-away detector broadcasts the new status, with the same per-tab session ID, to all connected servers in parallel.
**Why:** Servers are independent and shouldn't have to coordinate among themselves — that would require cross-server discovery and trust. The client is already connected to all of them and can coordinate cheaply. See ADR-025.
**Tradeoff:** A user signed in from two different devices to the same server may briefly expose different session states, but aggregation gives a stable effective user status.

## Permissions

Presence status is public. Any authenticated user can see any other authenticated user's presence.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-012 (Notifications)
