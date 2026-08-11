# FDR-036: Pinned Messages

**Status:** Active
**Last reviewed:** 2026-08-11

## Overview

Pinned Messages let room managers keep useful channel messages within easy
reach. Every channel member can browse the current pins in a room sidebar and
jump to the original message or thread. Direct-message rooms do not support
pins.

## Behavior

- A member with effective `room.manage` may pin or unpin any current message in
  a channel, including a thread reply. Pins do not introduce a separate
  permission.
- Any current channel member may list pins. Leaving or otherwise losing access
  to the room immediately removes its pinned-message state from the client.
- Pins appear newest-first in an automatically paginated **Pins** sidebar tab.
  The sidebar renders the complete message with the shared timeline message
  view; it does not apply Search's result-length clamp.
- Selecting a pin's jump action opens its canonical message. Pinning a visible channel
  echo pins the original thread reply so the association has one stable
  identity.
- A message may be pinned once per room. Repeating pin or unpin operations is a
  successful no-op. Editing a pinned message updates what the pin renders;
  retracting it removes its active pin.
- A dot on the room-header pin icon indicates that this device has not opened
  the Pins tab since a newer pin arrived. Opening the tab clears the dot. This
  marker is device-local, is not a server read receipt, and never opens the tab
  automatically.
- Archived rooms reject pin changes. They retain their existing pins for
  historical rendering while members can still read the room.

## Design Decisions

### 1. Reuse `room.manage`

**Decision:** Pin mutations require effective `room.manage`; there is no
`message.pin` permission.
**Why:** Pinning curates a room for all members and already fits the room
manager role. A new permission would add configuration surface without a
demonstrated need.
**Tradeoff:** Communities cannot delegate pinning independently from other room
management until a concrete use case justifies a narrower permission.

### 2. Store associations as room facts

**Decision:** `MessagePinnedEvent` and `MessageUnpinnedEvent` durably record the
canonical message ID and actor. Room Timeline projects the current pin set.
**Why:** Pins are shared room state that must replay consistently across
replicas. Keeping message content in the existing timeline projection avoids a
second content copy and makes edits immediately visible.
**Tradeoff:** Reading a pin page hydrates association metadata with canonical
message and user state.

### 3. Fence authorization and room state together

**Decision:** Mutations capture both the authorization fence and full room
aggregate tail, rerun `room.manage`, and append through OCC with bounded
retries.
**Why:** Separate replicas must not commit a pin after concurrent permission,
room lifecycle, message retraction, or pin-state changes invalidate the
decision.
**Tradeoff:** Unrelated writes in the same room may cause a retry, matching the
existing room mutation boundary.

### 4. Reuse the existing realtime operation

**Decision:** Pin events emit the established `server_state_upsert` realtime
operation with an additive `pinned_message_change` field.
**Why:** Existing clients already know how to safely process and ignore unknown
fields on this operation, while current clients receive an ordered pin refresh
without another top-level operation.
**Tradeoff:** A pin change causes the retained room pin store to refresh its
canonical page rather than applying an untrusted partial message payload.

### 5. Keep unseen state local

**Decision:** The client compares the newest server pin timestamp with a
device-local last-opened timestamp.
**Why:** The dot is a lightweight navigation hint, not shared notification
state. Avoiding a per-user server record keeps the feature simple and prevents
opening Pins on one device from changing another.
**Tradeoff:** A newly used browser may mark existing pins as unseen until the
tab is opened once.

## Compatibility

The RoomService RPCs, pinned-message resource, persisted event variants,
snapshot field, and realtime change field are additive protobuf changes. A
bounded batch-get RPC resolves authoritative pin state for messages currently
rendered by the client without making paginated list responses grow with the
room's entire pin set. The bundled client exposes the feature only for servers
at `0.5.0-0` or newer.
Older clients can continue processing `server_state_upsert` and ignore its new
field. Older servers return an unimplemented RPC, which gated clients do not
call. Persisted message events are additive and the disposable Room Timeline
snapshot schema receives a new fingerprinted contract namespace automatically.

## Related

- **ADRs:** ADR-016 (OCC for message publishing), ADR-033 (event-sourced state), ADR-045 (public API stability), ADR-050 (projection snapshots), ADR-051 (resumable client projection)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-004 (Message Editing & Deletion), FDR-019 (Room Lifecycle), FDR-031 (Client–Server Compatibility Discovery), FDR-033 (Message Search)
- **Issue:** [#1982](https://github.com/chattocorp/chatto/issues/1982)

## Open Questions

- Whether demonstrated community needs should later introduce a narrower pin
  permission or a configurable pin limit. Neither is part of the initial
  feature.
