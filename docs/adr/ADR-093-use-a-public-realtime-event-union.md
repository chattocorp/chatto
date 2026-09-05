# ADR-093: Use a Public Realtime Event Union with Dedicated Payloads

**Status:** Accepted

**Date:** 2026-09-03

**Refines:** The public event decision in
[ADR-091](ADR-091-semantic-realtime-events-with-bounded-resume.md).

## Context

An unpublished protocol 4 iteration put `chatto.core.evt.v1.Event` directly
inside the public realtime wrapper. The server sent a new authorized copy, not
stored bytes. A runtime allowlist selected the public variants, and custom
protobuf field options selected the public fields.

This design shared all payload messages, but it made the complete internal EVT
union part of the generated public type. An API reader could not use the schema
alone to learn which variants were public. A new internal event also appeared
in the public type before the realtime boundary had made an exposure decision.
The runtime allowlist and generated catalogue had to remain synchronized.

The field options also put public API policy on every stored field. They were
hard to audit and easy to apply incorrectly to nested messages. For example, a
public top-level field could make a nested storage pointer public through
inherited rules. The generated public API still showed the complete stored
message, including fields that the server removed at runtime.

A dedicated public payload catalogue does not need to copy every durable EVT
fact. Several related internal facts can map to one public change event. Public
names and compact union numbers can stay independent from the internal source.
Payload layouts can evolve independently from stored facts. Tests can enforce
the catalogue relationship and mapper coverage.

## Decision

`chatto.core.evt.v1.Event` is the internal envelope for stored EVT facts.
`chatto.core.pubsub.v1.PubSubEvent` is the internal envelope for NATS Core
signals. Existing stored event tags, payload messages, and bytes do not
change. See ADR-094.

The realtime API uses `chatto.realtime.v1.RealtimeEvent`. This message
contains:

- the canonical event ID, source time, and actor ID; and
- an explicit `event` oneof that contains only public variants; and
- an optional opaque resume cursor outside the payload oneof.

Each public union member references a dedicated payload in
`chatto/realtime/v1/events.proto`. Its name describes the public semantic event,
and its field number is independent from the internal source. Retired public
tags remain reserved. The
public payload has its own field numbers and contains only fields that clients
can receive.

`chatto.realtime.v1.RealtimeServerFrame` is the transport wrapper. Its `event`
arm contains one `RealtimeEvent` directly. No second event wrapper exists.
The cursor and common metadata remain outside the payload oneof. A client can
therefore ignore an unknown additive payload and still accept the complete
event boundary.

The server uses explicit typed mapping to admit approved values into a new
public event. Durable EVT facts can map to a detailed public event or to one
resource-change hint. Client-facing `PubSubEvent` variants reference the public
payload type directly, so the mapper only selects the matching public union
member. It deep-copies the result before caller-specific filtering. If the
public union has no matching member, the server omits the event. The public
union and mapper are the complete exposure catalogue. There are no field
surface options and no payload wire transcoding.

The public catalogue describes useful client semantics, not internal audit
detail. For example, profile facts map to `UserProfileChangedEvent`, server
branding facts map to `ServerProfileChangedEvent`, and private viewer
preferences map to `ViewerPreferencesChangedEvent`. The client reads the
canonical resource at the event cursor. Public moderation events do not expose
the moderator, reason, or internal membership operation. An effective return
to universal-room membership appears as an ordinary join.

The server applies current event authorization before it resolves plaintext or
maps the event. Public `_plaintext` fields exist only in the realtime payloads.
The mapper applies only trusted decrypted values. Ciphertext,
nonces, storage pointers, private moderation data, credentials, and other
internal fields do not exist in the public payload schema.

Room-group and layout facts map to one data-free `RoomLayoutChangedEvent`.
Clients read the current authorized layout through the resource API. The hint
contains no room IDs, links, or ordering data, so delivery does not need a
second layout-field authorization path. Asset processing completion similarly
carries asset and message IDs, not a second processed-video resource tree.

Posted messages include the immutable room kind, a named thread-root reference,
and structured mention causes. Integrations can classify a DM without a room
directory read. Edits and retractions name the affected message explicitly.

Event authorization must make every delivered field safe for the viewer.
Future payloads with narrower field visibility need an explicit viewer-aware
mapping rule, a separate authorized shape, or omission. This version does not
need general field-authorization machinery. Do not put that policy in
annotations on stored EVT fields.

Realtime protocol version 4 keeps this refined shape. Protocol 4 is not yet in
a Chatto release. The pull request that introduces it updates the server,
bundled client, generated clients, and public documentation together.

The public payload bytes are independent from EVT bytes. The private pubsub
envelope can carry the same public payload submessage, but it is not a public
transport wrapper. Clients must regenerate when the public schema changes.
Their type system exposes only the public contract.

## Consequences

- EVT and transient NATS Core signals use separate internal envelopes.
- Client-facing pubsub variants reuse public payload messages without exposing
  the private pubsub envelope.
- Realtime has one public event shape and one dedicated public payload
  catalogue.
- `RealtimeServerFrame` provides the transport wrapper around the event.
- The protobuf schema shows only client-visible event fields.
- Adding a public event requires a dedicated payload, one public union member,
  and explicit mapper coverage. A pubsub source also needs one restricted
  private union member that references that payload.
- Adding an internal event does not enlarge the public API.
- Current event authorization and the public payload schema define exposure.
- Layout and asset changes refer clients to canonical resources instead of
  duplicating their data in events.
- Public union evolution and stored EVT envelope evolution can be reviewed as
  separate compatibility decisions.
- Descriptor tests fail if public union alignment or mapper coverage drifts.
- Generated clients and reference documentation include the dedicated public
  payload catalogue.
