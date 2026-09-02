# ADR-091: Use One Event Vocabulary for Storage, Live Delivery, and Realtime

**Status:** Accepted
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
durable variants. It rejects transient variants and populated client-only
fields.

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

### Public delivery uses an authorized Event copy

`chatto.realtime.v1.RealtimeEvent` is a transport wrapper. It contains:

- one authorized canonical `Event`;
- an optional opaque resume cursor.

The server never sends raw stored bytes and never mutates a stored event. It
creates a new delivery object, checks event-level authorization, and copies only
fields that their protobuf options permit. Unknown fields are not copied.
Internal event variants are omitted by a server-owned public catalogue.

The custom protobuf field option `chatto.core.event.v1.event_field_surface`
classifies fields as:

- `SHARED`: allowed in storage and authorized delivery;
- `STORAGE_ONLY`: allowed in storage and removed from delivery;
- `CLIENT_ONLY`: rejected by storage and allowed in authorized delivery; or
- `UNSPECIFIED`: allowed in EVT, but denied at a public event-payload root.

A classified shared message field includes its ordinary nested value fields.
An explicit nested classification still overrides that inherited surface. This
keeps reusable value messages practical while storage-only fields remain
visible in code review.

Field surfaces are static exposure classes. They do not express viewer-specific
policy. Event-level authorization must therefore be sufficient for every
`SHARED` or populated `CLIENT_ONLY` field in that event. If a future field is
visible to only some authorized viewers, Chatto must add an explicit
viewer-aware projection rule or use a separate authorized resource or event.

Encrypted fields use an `_encrypted` or `encrypted_` name when the existing
stored compatibility contract permits that name. Delivery-only decrypted
companions use the `_plaintext` suffix. The server decrypts the exact source
event into a delivery-only clone before field projection. If key shredding has
removed the key, the plaintext field stays absent. Ciphertext, nonces, password
hashes, credential verifiers, provider subjects, and similar storage data do
not cross the public boundary.

`MessagePostedEvent.body_plaintext` lets an authorized client render a new
message without a second transport shape or a blocking resource read. It is
absent in EVT. The client can still read the canonical message resource to get
attachments, reactions, thread metadata, and timeline cursors. A read caused by
a durable event uses that event's cursor as its minimum boundary. The client
does not save the event cursor until the required read succeeds.

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

### Protocol version 4

Behavioral realtime protocol version 4 carries the canonical Event. It rejects
older handshakes. The protobuf package remains `chatto.realtime.v1`; that suffix
is a namespace and is not the behavioral protocol version.

## Compatibility

This change is additive for stored EVT bytes. Existing durable field tags and
payload fields keep their numbers, types, and oneof structure. New transient
oneof tags do not alter old records. New optional plaintext fields are absent in
old and current stored events, and the EVT boundary rejects attempts to store
them.

The public realtime change is intentionally breaking. Protocol negotiation
makes old and new clients fail at the handshake instead of interpreting the
wrong event shape.

The transient NATS wire is canonical-only. This intentional source and rolling
wire break applies to unreleased 0.5 development versions. Stored EVT bytes are
not affected.

## Consequences

- Chatto has one semantic event vocabulary for durable facts, transient
  signals, resume, bots, and the bundled frontend.
- Adding an event no longer requires a second public event payload.
- Current-state bootstrap reuses canonical ConnectRPC resource messages in a
  small WebSocket snapshot wrapper instead of a parallel state hierarchy.
- The WebSocket replica captures the snapshot and event boundary together.
  Targeted cursor-bounded reads can use any replica without accepting content
  older than an event boundary.
- EVT compatibility strengthens the public event contract.
- Authorization and field projection remain explicit server work. Sharing a
  protobuf does not make every event or field public.
- Field annotations and the public event catalogue are security boundaries and
  require tests and review.
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
