# ADR-093: Separate Durable and Live Event Envelopes

**Status:** Accepted

**Date:** 2026-09-03

**Supersedes:** The single internal envelope decision in
[ADR-091](ADR-091-use-one-event-vocabulary-for-storage-live-and-realtime.md).

## Context

ADR-091 put durable EVT facts and transient NATS Core signals in one internal
`Event` union. Callers selected storage after they built the event. This gave
both paths one envelope, but it made durability a rule outside the type. It
also added a large transient-only number range and live payload imports to the
stored EVT schema.

ADR-092 later gave the public realtime API its own event union and payloads.
The public API no longer needs one internal envelope to provide one semantic
event catalogue. The durable and transient internal paths have different
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

`chatto.core.live.v1.LiveEvent` contains only transient signals that are sent
on `live.sync.>` through NATS Core. It is never stored in EVT. Live event field
numbers are local to `LiveEvent`; they do not use the public realtime union
numbers.

Both envelopes have the common event ID, creation time, and actor ID. Backend
publishers and event-delivery code use typed accessors so that code must select
the durable or transient source. Durable EVT replay cannot contain a
`LiveEvent`.

The public `chatto.realtime.v1.RealtimeEvent` union stays independent. The
server maps selected `Event` and `LiveEvent` variants to dedicated public
payloads after authorization. Every `LiveEvent` variant is public in the
current catalogue. An exhaustive descriptor test fails when a new live
variant does not have a public mapping. Durable internal events can remain
private.

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

## Consequences

- EVT compatibility remains isolated in the durable `Event` schema.
- A transient signal cannot enter EVT through the normal typed publisher.
- Live NATS Core wire changes need mixed-version review, but they do not add
  fields to the stored EVT union.
- The public realtime catalogue keeps one semantic view across both internal
  sources without importing either internal payload schema.
- A new public durable event needs an explicit public mapping. A new
  `LiveEvent` also fails the catalogue test until it has one.
- Realtime startup has fewer messages, frame types, and partial states.
- An atomic snapshot is larger than one resource-family frame, but it is
  bounded, compressed when useful, and cannot be partly accepted.
- A future protocol change that needs client negotiation must use a new
  behavioral protocol version or discovery metadata.
