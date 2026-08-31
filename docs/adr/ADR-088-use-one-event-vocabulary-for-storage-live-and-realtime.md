# ADR-088: Use One Event Vocabulary for Storage, Live Delivery, and Realtime

**Status:** Accepted
**Date:** 2026-08-31

**Supersedes:** The separate-public-schema rule in
[ADR-087](ADR-087-semantic-realtime-events-with-bounded-resume.md), and the
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

`chatto.core.live.v1.LiveEvent` remains only as a rolling-upgrade read format.
Current code does not publish it. The process-wide ingress can decode it and
convert it to the canonical Event until the rolling compatibility window ends.

### Public delivery uses an authorized Event copy

`chatto.realtime.v1.RealtimeEvent` is a transport wrapper. It contains:

- one authorized canonical `Event`;
- an optional opaque resume cursor; and
- optional authorized current-state items.

The server never sends raw stored bytes and never mutates a stored event. It
creates a new delivery object, checks event-level authorization, and copies only
fields that their protobuf options permit. Unknown fields are not copied.
Internal event variants are omitted by a server-owned public catalogue.

The custom protobuf field option `chatto.core.event.v1.event_field_surface`
classifies fields as:

- `SHARED`: allowed in storage and authorized delivery;
- `STORAGE_ONLY`: allowed in storage and removed from delivery;
- `CLIENT_ONLY`: rejected by storage and allowed in authorized delivery; or
- `UNSPECIFIED`: denied at an event-payload boundary.

A classified shared message field includes its ordinary nested value fields.
An explicit nested classification still overrides that inherited surface. This
keeps reusable value messages practical while storage-only fields remain
visible in code review.

Encrypted fields use an `_encrypted` or `encrypted_` name when the existing
stored compatibility contract permits that name. Delivery-only decrypted
companions use the `_plaintext` suffix. The server decrypts the exact source
event into a delivery-only clone before field projection. If key shredding has
removed the key, the plaintext field stays absent. Ciphertext, nonces, password
hashes, credential verifiers, provider subjects, and similar storage data do
not cross the public boundary.

### Resume stays a transport concern

The public cursor remains encrypted, authenticated, viewer-bound, and opaque.
It can contain an EVT sequence internally. The canonical Event never contains
a JetStream sequence, subject, stream identity, or cursor.

Snapshots and event state use transport sidecars. They do not change the Event
shape and do not become synthetic domain events.

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

## Consequences

- Chatto has one semantic event vocabulary for durable facts, transient
  signals, resume, bots, and the bundled frontend.
- Adding an event no longer requires a second public event payload.
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
- [ADR-087](ADR-087-semantic-realtime-events-with-bounded-resume.md)
