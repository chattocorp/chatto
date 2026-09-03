# ADR-094: Separate Durable and Pubsub Event Envelopes

**Status:** Accepted

**Date:** 2026-09-03

**Restores and refines:** The internal storage-contract boundary in
[ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md).

## Context

An unpublished protocol 4 iteration put durable EVT facts and transient NATS
Core signals in one internal `Event` union. Callers selected storage after they
built the event. This gave both paths one envelope, but it made durability a
rule outside the type. It also added a large transient-only number range and
live payload imports to the stored EVT schema.

ADR-093 gives the public realtime API its own event union and payloads.
The public API no longer needs one internal envelope to provide one semantic
event catalogue. The durable and pubsub internal paths have different
compatibility and recovery rules, so separate envelopes now give a clearer
boundary.

The first protocol 4 frame model also used a two-message client handshake,
separate `error` and `close` frames, and several snapshot wrapper messages.
Protocol 4 is not in a Chatto release. These frames add states that do not
change the result of a subscription.

## Decision

`chatto.core.evt.v1.Event` contains only durable facts that can be stored in
EVT. Its existing stored field numbers, payloads, and wire layout stay
compatible with supported Chatto data.

`chatto.core.pubsub.v1.PubSubEvent` contains only transient signals. Most
signals are sent on `live.sync.>` through NATS Core. The process-local presence
hub also uses this envelope when it fans out a presence change. A
`PubSubEvent` is never stored in EVT. Its field numbers are local to this
envelope; they do not use the public realtime union numbers. Variants that
exist for authorized client delivery reference their `chatto.realtime.v1`
payload type directly. Private control variants keep a private payload.
Session termination is the current private control variant.

Both envelopes have the common event ID, creation time, and actor ID. Backend
publishers use typed, scope-specific accessors. A user-scoped publisher accepts
only user signals, and a room-scoped publisher accepts only typing signals with
a matching room ID. Consumers validate the subject and payload scope before
authorization. Durable EVT replay cannot contain a `PubSubEvent`.

The public `chatto.realtime.v1.RealtimeEvent` union stays independent from both
internal envelopes. The server maps selected `Event` facts into dedicated
public payloads after authorization. The restricted `PubSubEvent` union uses
those public payload types and maps them into a fresh, deep-copied
`RealtimeEvent`. Public field names and compact field numbers do not expose the
internal source. An exhaustive descriptor test fails when a public variant has
no mapping. Internal events can remain private. Internal control events can map
to protocol frames. In particular, session termination maps to `RealtimeClose`
and is not a public event.

The protocol 4 startup has one client message. The client sends
`RealtimeSubscribe` as the first binary WebSocket message and sends no more
application messages. The server sends only these frame types:

- `snapshot`, as one atomic bounded content value when snapshot fallback is
  needed;
- `event`, for one authorized public event;
- `caught_up`, for the recovery-to-live boundary;
- `heartbeat`, for liveness and safe cursor progress; and
- `close`, for all terminal protocol and session results.

The server does not send `hello`, `subscribed`, `error`, or `pong` frames.
WebSocket control frames provide transport ping and pong behavior. The client
learns the selected recovery path from the frames that it receives: a
snapshot starts snapshot recovery, replay starts with events, and
`caught_up` completes either path.

## Compatibility

The EVT envelope and every stored payload keep their existing wire shapes. The
storage compatibility check continues to compare them with `origin/main`.

The public realtime and private pubsub schemas are intentionally breaking. They
have not shipped in a Chatto release. Older clients cannot use protocol 4 on a
server with this schema, and newer clients cannot use the earlier development
form of protocol 4. Deploy the server and bundled frontend from the same build.
Integrations must regenerate their clients.

The pubsub package rename and compact numbering also prevent mixed application
replicas from decoding every `live.sync.>` message. Upgrade all server replicas
together. A missed pubsub event does not affect EVT or stored state. Current
resource reads and a new snapshot restore the related client state.

## Consequences

- EVT compatibility remains isolated in the durable `Event` schema.
- A pubsub event cannot enter EVT through the normal typed publisher.
- Pubsub NATS Core wire changes need mixed-version review, but they do not add
  fields to the stored EVT union.
- The public realtime catalogue keeps one semantic view across both internal
  sources without importing either internal envelope.
- Client-facing pubsub variants do not duplicate public payload declarations.
- A new public event needs an explicit public mapping from its internal source.
- A new client-facing `PubSubEvent` must reference a payload in the public
  catalogue and have an explicit public union mapping.
- Realtime startup has fewer messages, frame types, and partial states.
- An atomic snapshot is larger than one resource-family frame, but it is
  bounded, compressed when useful, and cannot be partly accepted.
- A future protocol change that needs client negotiation must use a new
  behavioral protocol version or discovery metadata.
