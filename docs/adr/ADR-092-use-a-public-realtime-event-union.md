# ADR-092: Use a Public Realtime Event Union with Canonical Payloads

**Status:** Accepted

**Date:** 2026-09-03

**Supersedes:** The public-envelope part of
[ADR-091](ADR-091-use-one-event-vocabulary-for-storage-live-and-realtime.md).
ADR-091 still defines the canonical internal envelope for durable and
transient events.

## Context

ADR-091 put `chatto.core.evt.v1.Event` directly inside the public realtime
wrapper. The server sent a new authorized copy, not stored bytes. A runtime
allowlist selected the public variants, and field options selected the public
fields.

This design shared all payload messages, but it made the complete internal EVT
union part of the generated public type. An API reader could not use the schema
alone to learn which variants were public. A new internal event also appeared
in the public type before the realtime boundary had made an exposure decision.
The runtime allowlist and generated catalogue had to remain synchronized.

A second set of payload messages would make the public boundary clear, but it
would recreate the duplicate semantic vocabulary that ADR-091 removed.

## Decision

`chatto.core.evt.v1.Event` remains the canonical internal envelope for stored
EVT facts and transient NATS Core signals. Existing stored event tags, payload
messages, and bytes do not change.

The realtime API uses `chatto.realtime.v1.RealtimeEvent`. This message
contains:

- the canonical event ID, source time, and actor ID; and
- an explicit `event` oneof that contains only public variants; and
- an optional opaque resume cursor outside the payload oneof.

Each public union member references the existing canonical payload message.
It also uses the same oneof field number and name as the matching canonical
event. The public API does not duplicate payload messages.

`chatto.realtime.v1.RealtimeServerFrame` is the transport wrapper. Its `event`
arm contains one `RealtimeEvent` directly. No second event wrapper exists.
The cursor and common metadata remain outside the payload oneof. A client can
therefore ignore an unknown additive payload and still accept the complete
event boundary.

The server maps a canonical event to the public union by its protobuf field
number. It verifies that the canonical and public payload types match. If the
public union has no matching member, the server omits the event. The public
union is therefore the event-level exposure catalogue. There is no separate
runtime allowlist.

Field-surface options remain the field-level exposure catalogue because the
public union reuses canonical payload messages. The server creates a fresh
payload, applies current authorization, copies `SHARED` and `CLIENT_ONLY`
fields, and omits `STORAGE_ONLY` and unknown fields.

Realtime protocol version 4 keeps this refined shape. Protocol 4 is not yet in
a Chatto release. The pull request that introduces it updates the server,
bundled client, generated clients, and public documentation together.

The nested wire bytes stay identical for existing public variants because the
metadata and payload fields keep their canonical numbers and types. Changing
the declared message type is still a generated-source API change. Clients must
regenerate so their type system exposes only the public union.

## Consequences

- EVT and internal NATS code continue to use one canonical envelope.
- Realtime has one public event shape and no duplicate payload schema.
- `RealtimeServerFrame` provides the transport wrapper around the event.
- The protobuf schema shows the complete public event catalogue.
- Adding a public event requires one mechanical union member with the same
  name, number, and payload type as the canonical member.
- Adding an internal event does not enlarge the public API.
- Public union evolution and stored EVT envelope evolution can be reviewed as
  separate compatibility decisions.
- Existing public variant bytes keep the canonical nested wire encoding.
- Descriptor tests and documentation generation fail if a public member does
  not match its canonical member.
