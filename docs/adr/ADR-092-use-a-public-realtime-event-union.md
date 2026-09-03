# ADR-092: Use a Public Realtime Event Union with Dedicated Payloads

**Status:** Accepted

**Date:** 2026-09-03

**Supersedes:** The public-envelope part of
[ADR-091](ADR-091-use-one-event-vocabulary-for-storage-live-and-realtime.md).
ADR-093 supersedes ADR-091 for the internal durable and transient envelopes.

## Context

ADR-091 first put `chatto.core.evt.v1.Event` directly inside the public
realtime wrapper. The server sent a new authorized copy, not stored bytes. A
runtime allowlist selected the public variants, and custom protobuf field
options selected the public fields.

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

A dedicated public payload catalogue duplicates protobuf declarations, but it
does not need a second semantic event model. Event names and union numbers can
stay aligned with canonical events. Payload layouts can evolve independently.
Tests can enforce the catalog relationship and mapper coverage.

## Decision

`chatto.core.evt.v1.Event` is the internal envelope for stored EVT facts.
`chatto.core.live.v1.LiveEvent` is the internal envelope for transient NATS
Core signals. Existing stored event tags, payload messages, and bytes do not
change. See ADR-093.

The realtime API uses `chatto.realtime.v1.RealtimeEvent`. This message
contains:

- the canonical event ID, source time, and actor ID; and
- an explicit `event` oneof that contains only public variants; and
- an optional opaque resume cursor outside the payload oneof.

Each public union member references a dedicated payload in the
`chatto/realtime/v1` event files. It uses the same oneof field number and name
as the matching canonical event. The public payload has its own field numbers
and contains only fields that clients can receive.

`chatto.realtime.v1.RealtimeServerFrame` is the transport wrapper. Its `event`
arm contains one `RealtimeEvent` directly. No second event wrapper exists.
The cursor and common metadata remain outside the payload oneof. A client can
therefore ignore an unknown additive payload and still accept the complete
event boundary.

The server uses an exhaustive typed switch to copy approved values from a
canonical event into a new public event. If the public union has no matching
member, the server omits the event. The public union and mapper are the complete
exposure catalogue. There are no field-surface options and no payload wire
transcoding.

The server applies current event authorization before it resolves plaintext or
maps the event. Public `_plaintext` fields exist only in the realtime payloads.
The mapper applies only trusted decrypted values. Ciphertext,
nonces, storage pointers, private moderation data, credentials, and other
internal fields do not exist in the public payload schema.

The delivery boundary also applies current caller-specific field
authorization. Room-group membership and order events can contain room IDs
that the caller cannot see. The server omits a single-room event for a hidden
room and removes hidden room references from complete-order events. This rule
uses the same room visibility source as the resource APIs. Future payloads
with caller-specific fields must extend this explicit delivery policy or omit
the field or event. Do not put this policy in annotations on stored EVT
fields.

Realtime protocol version 4 keeps this refined shape. Protocol 4 is not yet in
a Chatto release. The pull request that introduces it updates the server,
bundled client, generated clients, and public documentation together.

The public payload bytes are independent from EVT bytes. Clients must
regenerate when the public schema changes. Their type system exposes only the
public contract.

## Consequences

- EVT and transient NATS Core signals use separate internal envelopes.
- Realtime has one public event shape and one dedicated public payload
  catalogue.
- `RealtimeServerFrame` provides the transport wrapper around the event.
- The protobuf schema shows only client-visible event fields.
- Adding a public event requires a dedicated payload, one union member, and an
  explicit mapper case.
- Adding an internal event does not enlarge the public API.
- Caller-specific field authorization is explicit at the delivery boundary.
- Public union evolution and stored EVT envelope evolution can be reviewed as
  separate compatibility decisions.
- Descriptor tests fail if public union alignment or mapper coverage drifts.
- Generated clients and reference documentation include the dedicated public
  payload catalogue.
