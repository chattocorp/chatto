# FDR-037: Pinned Messages

**Status:** Active
**Last reviewed:** 2026-10-12

## Overview

Pinned Messages let authorized members keep useful channel messages within easy
reach. A channel member with a message-read mode can browse the current visible
pins in a room sidebar and jump to the original message or thread.
Direct-message rooms do not support pins.

## Behavior

- A member with effective `message.pin` may unpin any current message. Pinning
  also requires read access to the target message because the operation returns
  the hydrated pinned message. The permission applies at server, room-group,
  and room scope and does not require privileged mode. Neither `room.manage`
  nor `message.manage` grants it. New servers grant it to Admin and Moderator.
- A current channel member with `message.read` may list every pin. A member
  with `message.read-interactions` may list pins only from related threads.
  Losing membership or read authority immediately removes inaccessible
  pinned-message state from the client.
- Pins appear newest-first in the **Pinned messages** section of the **Room
  information** sidebar tab. The **General** section shows the
  room name, relative creation time, and live member count, including effective
  members of universal rooms. Creator identity is not exposed by the room API.
  The exact creation time is available on hover. Missing creation metadata is
  shown as unknown.
  A separate **Description** section follows General when the room has a description.
  It uses the same Markdown rendering as messages and the room join screen.
  All sections start expanded and can be collapsed
  independently. This device remembers the section preferences for each server.
  The pin list loads more messages automatically as you scroll.
- Selecting a pinned message opens its canonical message. The entire message row
  is the navigation target, while links and normal message actions keep their
  own behavior. Pinning a visible channel echo pins the original thread reply
  so the association has one stable identity.
- A message may be pinned once per room. Repeating pin or unpin operations is a
  successful no-op. Editing a pinned message updates what the pin renders;
  retracting it removes its active pin.
- A dot on the room-header information icon indicates that this device has not opened
  the Room information tab since a newer pin arrived. Opening the tab clears the dot. This
  marker is device-local, is not a server read receipt, and never opens the tab
  automatically. The dot is hidden when the room has no current pins, including
  after a live unpin removes the last unseen pin.
- Archived rooms reject pin changes. They retain their existing pins for
  historical rendering while members can still read the room.

## Design Decisions

### 1. Separate pin management from room management

**Decision:** Pin mutations require effective `message.pin`; pin creation also
requires read access to the target message. One permission controls both
pinning and unpinning, including pins created by another member.
**Why:** Operators can delegate content management without granting control
over room settings, permissions, or lifecycle. See issue #2924.
**Tradeoff:** Operators must configure another permission. Existing servers
must grant it explicitly to the roles or accounts that need pin access.

### 2. Store associations as room facts

**Decision:** `MessagePinnedEvent` and `MessageUnpinnedEvent` durably record the
canonical message association. Room Timeline projects the current pin set.
**Why:** Pins are shared room state that must replay consistently across
replicas. Keeping message content in the existing timeline projection avoids a
second content copy and makes edits immediately visible.
**Tradeoff:** Reading a pin page hydrates the canonical message and its normal
render dependencies.

### 3. Validate authorization and guard room state

**Decision:** Mutations validate stable request-time authorization inputs,
capture the full room aggregate tail, rerun `message.pin`, and append through
room OCC with bounded retries.
**Why:** Stable authorization validation prevents a torn decision. Room OCC
protects room lifecycle, message retraction, and pin state.
**Tradeoff:** Unrelated writes in the same room may cause a retry, matching the
existing room mutation boundary.

### 4. Use a semantic pin event

**Decision:** Pin creation and deletion emit semantic public pin events with
the room and message references that clients need to refresh or update their
authorized pin view.
**Why:** The event describes the domain change directly for bots and the
frontend. Clients can ignore an event type they do not use and still advance
the common cursor. See ADR-091.
**Tradeoff:** A pin change causes the loaded room pin store to refresh its
canonical page rather than apply an untrusted partial message payload.

### 5. Keep unseen state local

**Decision:** The server orders pins by their durable event sequence and returns
an opaque marker for the latest pin fact even after that pin is removed. The
client compares that stable marker with the device-local marker
recorded when the Room information panel was last opened. Local state is scoped to the
authenticated viewer and removed when room access is revoked. The marker does
not expose or identify the pin operation or underlying stream sequence.
**Why:** The dot is a lightweight navigation hint, not shared notification
state. Avoiding a per-user server record keeps the feature simple and prevents
opening Room information on one device from changing another.
**Tradeoff:** A newly used browser may mark existing pins as unseen until the
tab is opened once. Because the comparison does not use wall-clock timestamps,
replica clock skew cannot suppress the indicator or change canonical pin order,
and removing the newest pin while a client is offline does not create a false
"new pins" indication.

### 6. Keep pin lists bounded

**Decision:** Pin lists use the standard page request and response metadata;
the sidebar loads subsequent pages automatically.
**Why:** Rooms do not impose an arbitrary pin limit, and every row contains a
full renderable message with attachments, reactions, and thread metadata.
Pagination keeps the API response bounded without adding visible controls.
**Tradeoff:** Clients that need the entire pin set must follow pagination even
though most rooms will fit in the first page.

## Compatibility

Pin clients must use the room viewer state's `message.pin` grant instead of
`room.manage` to show pin controls. Existing permission decisions and pins
remain stored unchanged. Existing servers must grant `message.pin` explicitly;
startup does not change their role decisions.

The RoomService RPCs and response marker, pinned-message resource,
`Message.pinned` field, persisted event variants, snapshot fields, and realtime
change field are additive protobuf changes. Normal timeline, singular message,
and batched message reads all expose authoritative pin state without a parallel
pin-status lookup. The bundled client supports only servers that include the
feature (see FDR-031).
Older clients can continue processing `server_state_upsert` and ignore its new
field. Persisted message events are additive and the disposable Room Timeline
snapshot schema receives a new fingerprinted contract namespace automatically.

## Permissions

- `message.read` — list all room pins and create a pin whose response contains
  the hydrated message.
- `message.read-interactions` — list pins from related threads and create a pin
  for a readable related message.
- `message.pin` — create or delete pin associations.

## Related

- **ADRs:** ADR-016 (OCC for message publishing), ADR-033 (event-sourced state), ADR-045 (public API stability), ADR-050 (projection snapshots), ADR-080 (explicit message-read permissions), ADR-082 (derived thread interactions), ADR-087 (request-time authorization with aggregate OCC), ADR-089 (server content view), ADR-091 (semantic realtime events)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-004 (Message Editing & Deletion), FDR-019 (Room Lifecycle), FDR-031 (Client–Server Compatibility Discovery), FDR-033 (Message Search), FDR-039 (Message Access & Interactions), FDR-045 (Realtime Event Stream)
- **Issue:** [#1982](https://github.com/chattocorp/chatto/issues/1982)
- **Issue:** [#2924](https://github.com/chattocorp/chatto/issues/2924)

## Open Questions

- Whether communities need a configurable pin limit.
