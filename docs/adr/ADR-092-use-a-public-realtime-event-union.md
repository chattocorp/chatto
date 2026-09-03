# ADR-092: Use a Public Realtime Event Union with Dedicated Payloads

**Status:** Accepted

**Date:** 2026-09-03

**Supersedes:** The public-envelope part of
[ADR-091](ADR-091-use-one-event-vocabulary-for-storage-live-and-realtime.md).
ADR-091 still defines the canonical internal envelope for durable and
transient events.

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
does not need a second semantic event model. The event names, union numbers,
and shared field wire encodings can stay aligned with the canonical events.
Tests can enforce this relationship.

## Decision

`chatto.core.evt.v1.Event` remains the canonical internal envelope for stored
EVT facts and transient NATS Core signals. Existing stored event tags, payload
messages, and bytes do not change.

The realtime API uses `chatto.realtime.v1.RealtimeEvent`. This message
contains:

- the canonical event ID, source time, and actor ID; and
- an explicit `event` oneof that contains only public variants; and
- an optional opaque resume cursor outside the payload oneof.

Each public union member references a dedicated payload in
`chatto/realtime/v1/events.proto`. It uses the same oneof field number and name
as the matching canonical event. Shared payload fields also keep their
canonical names, numbers, types, cardinality, presence, and oneof membership.
The public payload contains only fields that clients can receive.

`chatto.realtime.v1.RealtimeServerFrame` is the transport wrapper. Its `event`
arm contains one `RealtimeEvent` directly. No second event wrapper exists.
The cursor and common metadata remain outside the payload oneof. A client can
therefore ignore an unknown additive payload and still accept the complete
event boundary.

The server maps a canonical event to the public union by its protobuf field
number and name. It serializes the canonical payload into a new public payload
and discards fields that the public message does not declare. If the public
union has no matching member, the server omits the event. The public union and
its payload file are therefore the complete exposure catalogue. There is no
second runtime allowlist and there are no field-surface options.

The server applies current authorization before it resolves plaintext or maps
the event. Public `_plaintext` fields exist only in the realtime payloads.
Their numbers and names are reserved in the matching EVT messages. The mapper
clears these fields before it applies trusted decrypted values. Ciphertext,
nonces, storage pointers, private moderation data, credentials, and other
internal fields do not exist in the public payload schema.

Realtime protocol version 4 keeps this refined shape. Protocol 4 is not yet in
a Chatto release. The pull request that introduces it updates the server,
bundled client, generated clients, and public documentation together.

The nested wire bytes stay compatible for shared fields because the metadata,
union members, and payload fields keep their canonical numbers and wire types.
The declared public message types are different, so clients must regenerate.
Their type system then exposes only the public contract.

## Consequences

- EVT and internal NATS code continue to use one canonical envelope.
- Realtime has one public event shape and one dedicated public payload
  catalogue.
- `RealtimeServerFrame` provides the transport wrapper around the event.
- The protobuf schema shows only client-visible event fields.
- Adding a public event requires a dedicated payload and one union member with
  the same name and number as the canonical member.
- Adding an internal event does not enlarge the public API.
- Public union evolution and stored EVT envelope evolution can be reviewed as
  separate compatibility decisions.
- Existing public variant bytes keep the canonical nested wire encoding.
- Descriptor tests fail if public names, numbers, field wire shapes, plaintext
  reservations, mapper coverage, or transient-event coverage drift.
- Generated clients and reference documentation include the dedicated public
  payload catalogue.
