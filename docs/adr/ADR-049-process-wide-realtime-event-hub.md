# ADR-049: Process-Wide Realtime Event Hub

**Date:** 2026-07-14

## Context

The realtime WebSocket receives durable facts from the `live.evt.>` EVT
republish and transient signals from `live.sync.>`. Each `StreamMyEvents`
connection previously created its own wildcard subscription to both roots.
NATS therefore copied every raw payload into every connection's queue, and
Chatto decoded it once per connection before deciding whether the event was
deliverable.

This multiplied global event traffic by the number of connected clients.
Private encrypted `message_body` facts were especially wasteful: their subject
already identified them as non-deliverable, but every connection retained and
decoded the payload before rejecting it. Each connection also maintained an
independent room-visibility cache and repeated identical projection waits and
RBAC-driven cache rebuilds for multiple sessions belonging to one user.

Realtime delivery is live-only. When a queue loses an event, the client must
reconnect and recover current state through projected reads; continuing a
healthy-looking but incomplete stream can leave authorization, room state, or
presence stale.

## Decision

Each Chatto process runs one `MyEventsHub` with one NATS Core subscription per
live root. The hub processes incoming messages in order and performs shared
work before session fanout:

1. Classify the NATS subject before protobuf decoding. Discard private and
   non-deliverable EVT families, including `message_body`, immediately.
2. Decode each remaining event once.
3. For deliverable EVT facts, wait once for the local projections required by
   authorization and subsequent projected reads.
4. Apply room, asset, user, and transient-event authorization per connected
   user. Sessions for the same user share one room-visibility cache.
5. Fan the same immutable decoded event envelope out to that user's independent
   session queues.

New sessions hydrate visibility without holding the dispatcher lock. Before
the read, the hub records the authoritative EVT tails for room-visibility and
RBAC facts, waits for the owning projections, and verifies that those tails
remained stable through hydration. Ordinary messages, reactions, and call facts
therefore cannot starve admission. Registration then crosses a dispatcher-owned
channel after draining the ingress messages already received by the process. If a
visibility-changing fact was processed while the snapshot was being built,
registration retries; if a pre-snapshot fact arrives late from another NATS
publisher or route, its EVT stream sequence identifies it as already reflected
and prevents it from mutating the newer cache. Correctness therefore does not
depend on global NATS publication order.

The dispatcher remains ordered. In particular, RBAC facts wait for the RBAC
projection and refresh every connected user's shared visibility state before a
later room event can be authorized. Membership facts mutate the shared state
before the next event is handled.

Session queues are bounded by both event count and the encoded bytes referenced
by that session. A session that exceeds either limit is disconnected without
blocking other sessions. A process-wide NATS ingress loss or projection-wait
failure disconnects all current sessions because the hub can no longer prove
that their live stream is continuous. The hub quarantines new admission,
unsubscribes and flushes the lossy live roots, drains their old backlog, and
only then opens a fresh ingress generation. Reconnecting connections rebuild
visibility from current projections and register in that new generation.

Presence continues to use its separate process-wide KV watcher because it is
latest-value runtime state rather than EVT or transient live-sync input. A
presence subscriber whose queue overflows is now marked lagged and closed so
the WebSocket reconnect path clears stale presence and hydrates current values.

The public `chatto.realtime.v1` protocol, EVT subjects and payloads, and
projected-read recovery contract do not change. Mixed-version clients continue
to observe the same authorized event shapes and reconnect behavior.

## Consequences

Realtime NATS subscription count, raw payload copies, protobuf decoding, and
projection-readiness waits scale with Chatto processes instead of connected
clients. Large private message-body facts no longer enter per-session queues or
the protobuf decoder on the realtime path. Multiple tabs or devices for one
user reuse the same room-visibility state.

Decoded protobuf messages are shared as immutable pointers. Future mapping or
filtering code must not mutate an event after hub publication.

One projection wait can temporarily hold up delivery for every connection on
the process. This preserves event order and is acceptable because projectors
normally lead or closely follow EVT republish. The wait remains bounded; a
failure causes explicit reconnect/catch-up instead of silent loss.

The hub is process-local and does not coordinate replicas. Every replica still
receives the singleton EVT republish, waits for its own projections, and
authorizes its own connected users. No correctness property depends on a
single Chatto replica.
