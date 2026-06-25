# FDR-011: User Presence

**Status:** Active
**Last reviewed:** 2026-06-25

## Overview

Every user has a presence status visible to others as a colored dot on their avatar: **Online**, **Away**, **Do Not Disturb**, or **Offline**. Presence is server-wide — a user has one status per Chatto server, not per space or room.

## Behavior

- Current clients report their own presence through `PresenceService.ReportPresence` on the ConnectRPC API. Legacy clients that do not opt into Connect-based presence reporting still get compatibility presence refreshes from the GraphQL `myEvents` subscription.
- In automatic mode, users start Online. After 5 minutes without keyboard/mouse/touch input, the client transitions to Away.
- If the browser tab is hidden for 10 seconds, the client also transitions to Away (debounced to avoid flashing during quick tab switches).
- Any interaction returns the user to Online only while automatic mode is active.
- Users can explicitly set Away. Explicit Away does not auto-return to Online on activity.
- Users can set Do Not Disturb for their current live server presence. While DND is active, new notifications are still recorded for that user, but notification sounds and web push are suppressed (see FDR-012). Presence state is not persisted as server-side user/account state.
- Explicit Away and Do Not Disturb are marked as manually selected in the live presence record. Automatic Online/Away reports from other clients do not overwrite that manual state; an explicit Online selection clears it.
- Users can choose "Look offline" locally. The client does not report an Offline status; it stops reporting presence to the server and pauses live event subscriptions so the existing presence record expires normally.
- Disconnecting (closing the tab, network drop) does not send an active Offline signal. After 60 seconds without a heartbeat refresh, the presence entry expires and the user appears Offline.
- The presence dot updates across the UI as other users' statuses change, in real time.

## Design Decisions

### 1. Server-wide, not per-space

**Decision:** A user has one presence status across all spaces/rooms in a server.
**Why:** Anything else is confusing — "I'm online in #design but away in #engineering" doesn't match how presence works in any other chat tool. Per-server matches the user's actual session.
**Tradeoff:** Users can't selectively appear online for some rooms. They can mute rooms for notification purposes (see FDR-012) but not for presence.

### 2. Offline is inferred, not stored

**Decision:** Offline is the absence of a live presence record, not a stored state. Current clients refresh the user's presence entry every 30 seconds through ConnectRPC; if all clients disconnect or choose "Look offline", the entry expires after 60 seconds via NATS KV TTL.
**Why:** A disconnecting client may not get the chance to send a clean "I'm offline" message (browser crash, network drop). Relying on TTL expiry handles all the failure modes uniformly.
**Tradeoff:** Up to a one-minute delay between "user closed the tab" and "the dot turns gray". This is the standard behavior in most chat apps and matches user expectations.

### 3. User-level live status with heartbeat-driven deduplication

**Decision:** Presence is stored in `MEMORY_CACHE` as `presence.{userId}`. A per-process PresenceHub watches these keys and emits live events only when the user-level status changes. Current clients write `ONLINE`, `AWAY`, or `DO_NOT_DISTURB` through `PresenceService.ReportPresence`; `OFFLINE` is not an accepted self-reporting value. The live record carries whether the status was manually selected so automatic reports from other clients cannot clear explicit Away/DND.
**Why:** Presence is a current-state hint, not durable account history, but explicit availability choices should not be defeated by another idle detector. Closing a tab does not actively write Offline, so another open tab can keep automatic presence alive after the manual TTL expires.
**Tradeoff:** "Look offline" remains client-local: another active browser/device can still keep the user visible because the invisible client deliberately does not tell the server about that privacy choice.

### 4. Auto-away has two triggers (idle + tab hidden)

**Decision:** Automatic mode has two independent triggers that transition to Away: 5 minutes of input inactivity, OR 10 seconds of tab hidden.
**Why:** Idle-only would miss the very common "switched tabs" case. Tab-hidden-only would mark people as away the moment they alt-tab to look at something. Combining both covers the realistic away cases.
**Tradeoff:** Some false positives — a user actively listening in another window is technically "away" by our model. Acceptable for the use case.

### 5. DND is live user state

**Decision:** Do Not Disturb is a live presence status for the user, not durable account state. It expires with presence and is not backed up or replayed from EVT. While present, it silences notification sounds and web push delivery without dropping the underlying notification records. Durable custom statuses live separately as user profile metadata (FDR-022).
**Why:** Presence controls notification routing and "right now" UI hints. Persisting it as domain/account history would overstate its meaning, while custom statuses communicate user-authored profile context without changing availability.
**Tradeoff:** The UI has two adjacent concepts: live presence dot and durable custom status. They deliberately answer different questions.

### 6. Invisible mode is client-local privacy behavior

**Decision:** "Look offline" is not a server status. The client stops self-reporting presence and stops receiving live event streams while the local mode is active. The server and other users only see the existing presence record expire.
**Why:** Reporting an explicit invisible/offline status would make the server aware of the user's privacy choice and could leak it to other clients or operators through logs, admin tooling, or future APIs. Absence of reporting preserves the privacy property.
**Tradeoff:** The user's own client loses realtime updates while invisible and must catch up from projected reads after returning to a reporting mode.

### 7. Per-server tracking, with frontend coordination across servers

**Decision:** Each connected Chatto server tracks its own presence. The frontend's auto-away detector broadcasts the new status to all connected servers in parallel.
**Why:** Servers are independent and shouldn't have to coordinate among themselves — that would require cross-server discovery and trust. The client is already connected to all of them and can coordinate cheaply. See ADR-025.
**Tradeoff:** A user signed in from two different devices to the same server may have competing presence writers; the latest write wins until TTL expiry.

## Permissions

Presence status is public. Any authenticated user can see any other authenticated user's presence.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-012 (Notifications), FDR-022 (User Profile)
