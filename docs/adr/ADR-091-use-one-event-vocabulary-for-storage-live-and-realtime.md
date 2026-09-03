# ADR-091: Use One Event Vocabulary for Storage, Live Delivery, and Realtime

**Status:** Superseded by
[ADR-092](ADR-092-use-a-public-realtime-event-union.md) for the public event
shape and by
[ADR-093](ADR-093-separate-durable-and-live-event-envelopes.md) for the
internal durable and transient envelopes.
**Date:** 2026-08-31

**Supersedes:** The separate-public-schema rule in
[ADR-090](ADR-090-semantic-realtime-events-with-bounded-resume.md), and the
separate durable and transient envelope rule in
[ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md). It also
supersedes the two-envelope part of
[ADR-012](ADR-012-two-tier-realtime-events.md).

## Context

Chatto had three event representations:

- `chatto.core.evt.v1.Event` for durable EVT facts;
- `chatto.core.live.v1.LiveEvent` for transient NATS Core signals; and
- duplicated semantic event messages in `chatto.realtime.v1`.

The public messages described the same changes as the internal events, but each
new event needed another mapping and another compatibility decision. Bounded
resume also had to translate stored events into a parallel event vocabulary.

EVT compatibility is already Chatto's strongest event compatibility contract.
Existing stored bytes must remain readable. Using the same semantic event
shape for public delivery can extend this strength to the realtime API without
exposing stored bytes, broker coordinates, or confidential fields.

## Decision

### One canonical Event envelope

`chatto.core.evt.v1.Event` is the canonical envelope for durable and transient
Chatto events. Backend callers still decide the durability:

- durable facts are written to EVT and republished on `live.evt.>`; and
- transient signals are published only through NATS Core on `live.sync.>`.

The envelope does not select storage. The storage publisher accepts only known
durable variants and rejects transient variants.

Existing durable oneof tags and payload wire shapes do not change. Transient
variants use tags 20000 through 29999. Tags 1000 through 9999 stay reserved for
retired legacy live variants, except the durable reaction tags 1050 and 1051.
High protobuf field numbers have a small key-size cost but no relevant runtime
or compatibility cost here.

Transient publishers serialize only the canonical Event. The old
`chatto.core.live.v1.LiveEvent` envelope is removed in the same atomic change.
Mixed old and new application replicas do not exchange transient
`live.sync.>` signals during a rolling replacement. Each side drops the
unknown envelope safely. Durable EVT delivery is unaffected. Reconnect
resource reads and credential revalidation restore authoritative current state.
Transient payload messages stay in `chatto.core.live.v1`; moving those symbols
would add source churn without changing the one-envelope runtime model.

### Public delivery experiment (superseded)

This section is superseded by ADR-092. It records the first protocol 4 design.

The first protocol 4 design used an authorized copy of the canonical event and
custom field-surface options. ADR-092 replaces that design. The field options
and client-only fields in EVT messages are removed. Realtime now owns dedicated
public payloads and plaintext fields. The canonical envelope remains internal.

### Exact snapshots and resume share one boundary

The public resume cursor is a signed, viewer-bound JWT. Its public `p` claim is
an HMAC of the EVT stream identity, viewer, subscription scope, and sequence.
The server compares `p` with at most 10,000 candidate positions in the retained
replay window. The canonical Event never contains a JetStream sequence,
subject, stream identity, or cursor.

When a client needs a complete current content view, it selects `SNAPSHOT` as
its fallback. The WebSocket replica registers for live events, captures an
exact `ServerContentView` snapshot at boundary `E`, and sends canonical public
resource messages. The snapshot contains the public server profile, visible
rooms, visible room groups, visible active calls, the viewer, and only users
that these resources reference. It does not contain the complete user
directory or large and paginated resources.

The server then sends `caught_up(E)`, drops buffered durable duplicates through
`E`, and continues live delivery. The client does not send a second catch-up
request. A client that does not need current resources selects `LIVE_ONLY`. A
client with a usable cursor receives bounded replay. An invalid, expired, or
unsafe cursor uses the selected `SNAPSHOT` or `LIVE_ONLY` fallback.

Normal event frames do not contain resource sidecars. Clients use ConnectRPC
to refresh a resource after an event when they need more than the semantic
payload. The client retains an event cursor only after these reads succeed.
This rule makes a failed read safe to retry after reconnect.

After an event, the client can set `Chatto-Realtime-Minimum-Cursor` to that
event's cursor on a targeted resource read. The serving replica validates the
viewer-bound token and waits until its content view includes at least that
boundary. This prevents a later read from another replica from returning older
content.

Room and thread history does not use realtime snapshot messages. Clients
read timelines through the paginated ConnectRPC services. This keeps large and
lazy data out of the WebSocket protocol.

### Protocol version 4 public shape (superseded)

This subsection records the first protocol version 4 design and is superseded
by ADR-092. That design carried the complete canonical `Event`. The current
protocol version 4 carries the explicit public `RealtimeEvent` union and its
dedicated payload messages. It rejects older handshakes. The protobuf package
remains `chatto.realtime.v1`; that suffix is a namespace and is not the
behavioral protocol version.

## Compatibility

This change is additive for stored EVT bytes. Existing durable field tags and
payload fields keep their numbers, types, and oneof structure. New transient
oneof tags do not alter old records. Plaintext fields exist only in the public
realtime payloads. Their former EVT names and numbers are reserved.

The public realtime change is intentionally breaking. Protocol negotiation
makes old and new clients fail at the handshake instead of interpreting the
wrong event shape.

The transient NATS wire is canonical-only. This intentional source and rolling
wire break applies to unreleased 0.5 development versions. Stored EVT bytes are
not affected.

## Consequences

- Chatto has one canonical internal event envelope for durable facts and
  transient signals.
- Realtime has a semantic one-to-one relationship with selected canonical
  events, but it owns dedicated public payload messages.
- Current-state bootstrap reuses canonical ConnectRPC resource messages in a
  small WebSocket snapshot wrapper instead of a parallel state hierarchy.
- The WebSocket replica captures the snapshot and event boundary together.
  Targeted cursor-bounded reads can use any replica without accepting content
  older than an event boundary.
- EVT compatibility informs the public event contract without exposing the EVT
  schema.
- Authorization and mapping remain explicit server work. The public payload
  schema is the field-level security boundary.
- The public catalogue and its exhaustive descriptor tests require review.
- The live payload package can be reorganized later, but that source-layout
  change is not required for one envelope and one semantic vocabulary.

## Related

- [ADR-008](ADR-008-protobuf-for-event-serialization.md)
- [ADR-012](ADR-012-two-tier-realtime-events.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-049](ADR-049-process-wide-realtime-event-hub.md)
- [ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md)
- [ADR-090](ADR-090-semantic-realtime-events-with-bounded-resume.md)
