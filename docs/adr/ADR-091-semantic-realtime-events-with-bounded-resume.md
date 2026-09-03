# ADR-091: Use Semantic Realtime Events with Bounded Resume

**Status:** Partially superseded by
[ADR-092](ADR-092-use-one-event-vocabulary-for-storage-live-and-realtime.md),
[ADR-093](ADR-093-use-a-public-realtime-event-union.md), and
[ADR-094](ADR-094-separate-durable-and-pubsub-event-envelopes.md). ADR-093
defines the public union and dedicated payload catalogue. ADR-094 defines the
current internal envelopes and protocol 4 frame set. The authorization,
bounded-resume, snapshot, and transport rules that this ADR introduced remain
active.

**Date:** 2026-08-30

**Supersedes:** [ADR-051](ADR-051-server-scoped-resumable-client-projection.md).
Partially supersedes [ADR-012](ADR-012-two-tier-realtime-events.md) for the
public realtime mapping. The internal durable and transient channel split from
ADR-012 remains in effect.

## Context

Realtime protocol 2 uses frontend projection mutations as its public unit.
This model gives the bundled frontend one ordered bootstrap and update path.
It also avoids many ConnectRPC reads during startup and reconnect.

The same model is difficult for bot authors and other integrators:

- Operations such as resource replacement and projection reset describe a
  frontend cache, not what happened in Chatto.
- A client cannot safely ignore an unknown projection mutation because that
  mutation can be required for convergence.
- Bots receive notification state, but they do not receive one clear public
  event vocabulary for messages, edits, retractions, reactions, membership,
  rooms, calls, and other authorized activity.
- Realtime protocol capabilities describe combinations of projection
  behavior that should instead be part of one protocol contract.

A live-only event stream plus ConnectRPC reads would be simpler for the server.
However, it would make the bundled frontend run many reads to rebuild its
server view. It would also restore races and stale retained state that the
server-scoped projection fixed.

An unlimited history cursor would create a different problem. The public feed
uses current authorization, deletion, and erasure state. Some current state,
such as presence and read markers, is not in EVT. An old cursor is therefore
not an audit-log position. Large global scans also need strict resource bounds.

One JetStream consumer for each client or catch-up is not an acceptable
scaling model. Chatto already has a process-wide live hub, and bounded direct
EVT reads can recover a recent connection gap without another consumer.

## Decision

### Invariants

The implementation and future protocol changes must preserve these invariants:

1. **The public stream is bot-shaped.** Public events describe authorized
   Chatto domain activity. They do not describe frontend cache operations.
2. **The frontend uses the public stream.** The bundled frontend does not get a
   private live event contract. It builds its projection from the same public
   events that bots and alternate clients receive.
3. **All public durable activity has one event path.** A durable change that
   has an authorized public meaning must be available as a semantic public
   event. Notification policy does not define the complete bot event set.
4. **Internal storage is not public API.** Raw EVT payloads, subjects, stream
   identities, and sequence numbers do not cross the public boundary.
5. **Authorization is evaluated at delivery.** Live delivery, resume, and
   snapshots use the caller's current permissions, membership, deletion, and
   erasure state. Historical access does not preserve revoked visibility.
6. **Snapshots and events have different meanings.** A snapshot states current
   authorized state. An event states a domain change. A snapshot row is not a
   synthetic historical event.
7. **Resume is bounded session recovery.** A cursor repairs an ordinary recent
   disconnect. It is not a promise of indefinite history or an audit log.
8. **Unsafe recovery is explicit.** A missing, invalid, expired, unsafe, or
   expensive cursor does not cause partial replay or an unbounded scan. A
   projection client receives an authorized snapshot. A live-only client starts
   at a stated current boundary and receives no historical events.
9. **Realtime clients do not own NATS consumers.** Live delivery uses the
   process-wide hub. Resume uses bounded direct EVT reads. Chatto does not
   create a JetStream consumer for each WebSocket or catch-up.
10. **Durable and transient delivery stay distinct.** Durable public events can
    resume from EVT. Typing, presence transitions, and other transient events
    are live-only. Current latest-value state converges through snapshots or
    reconciliation.
11. **Additive event variants are skippable.** Common event metadata and the
    cursor stay outside the event-variant `oneof`. A client can ignore an event
    variant that it does not use and still advance safely.
12. **Protocol versions define required behavior.** The protocol does not use
    a capability matrix to restate required frame semantics. A change that
    requires every client to behave differently uses a new behavioral protocol
    version.
13. **Public delivery does not expose stored bytes.** Per ADR-093, public
    delivery uses an explicit public union with dedicated payload messages.
    The server creates a fresh authorized value. Storage-only fields do not
    exist in its public schema.
14. **Long-offline reliable automation is separate.** If Chatto later promises
    eventual processing of every durable trigger, that promise uses an
    acknowledged webhook or paged activity contract. It does not change the
    realtime WebSocket into a durable per-client queue.

### Public realtime is a semantic event stream

The public realtime API is an authorized event stream for bots,
integrations, alternate clients, and the bundled frontend. The frontend is a
consumer of this public contract. Frontend cache operations do not define the
public event vocabulary.

Each durable domain change that has an authorized public meaning maps to a
semantic public event. Examples include message posts, edits, and retractions;
reaction changes; room and membership changes; profile changes; call changes;
and other public domain activity. Public events are not raw EVT messages. The
server creates authorized public events after projection readiness and
authorization checks.

A durable public event contains:

- a stable public event ID;
- the time of the source change;
- the actor when the caller can see it;
- an opaque resume cursor; and
- one semantic event variant with authorized resource data or resource
  references.

The cursor and common metadata stay outside the event-variant `oneof`. A client
can ignore an additive event variant that it does not use and still advance its
cursor. A new event variant must not change the meaning of an existing variant.
New behavior that requires all clients to act uses a new behavioral protocol
version.

Protocol capabilities do not duplicate the event schema. The behavioral
protocol version defines required wire behavior. Server configuration and
viewer permissions remain separate resource data.

### The frontend remains a client projection

The bundled frontend keeps one authenticated, server-scoped projection. A
subscription explicitly selects `SNAPSHOT` or `LIVE_ONLY` initial state. The
frontend selects `SNAPSHOT`. A simple bot can select `LIVE_ONLY` and use
ConnectRPC for resources that it needs. The snapshot avoids a fan-out of
startup resource requests. It uses one atomic snapshot frame with a bounded
set of resource families and keeps large collections lazy.

After the snapshot, the frontend applies semantic public events to its local
projection. ConnectRPC remains available for commands, explicit reads,
pagination, history, and data that the client loads on demand. A client does
not need the snapshot when it only wants new events.

Snapshot messages and semantic event messages have separate purposes. A
snapshot states what is current. An event states what changed. They can reuse
the same canonical public resource messages, but a snapshot row is not a
synthetic historical event.

### Resume is bounded session recovery

An authenticated client can reconnect with its last safe cursor. A usable
cursor resumes a recent connection gap in global EVT order and then joins live
delivery. Resume is not arbitrary historical playback and is not a public
audit log.

The cursor is a signed JWT that is bound to the viewer. Its public `p` claim is
an HMAC of the EVT stream identity, viewer, subscription scope, and global
sequence. It does not expose the sequence or other NATS and JetStream
coordinates. The server resolves `p` only within the bounded replay window.

The server applies current authorization, deletion, and erasure state during
resume. When the cursor is missing, invalid, expired, from another stream
incarnation, unsafe after an authorization change, or outside the configured
work budget, the server uses the subscription's requested fallback. A
`SNAPSHOT` subscription receives current state. A `LIVE_ONLY` subscription
starts at the current boundary. A snapshot before `caught_up` identifies the
fallback that the server used.

Resume uses the existing handoff pattern:

1. Register the connection with the process-wide live hub.
2. Capture a stable EVT boundary.
3. Wait for the required projections through that boundary.
4. Read the bounded EVT sequence range directly with point reads.
5. Map authorized public events and send them in order.
6. Mark the boundary as caught up.
7. Drop buffered duplicates through the boundary and continue live.

Realtime resume does not create a JetStream consumer for the client. Age,
sequence-span, event-count, concurrency, and time limits are operational
policy. Crossing a limit selects the requested safe fallback instead of an
unbounded scan. A cumulative byte limit remains a measured follow-up because
the current sequence, event-count, and time limits already bound initial work.

Transient events, such as typing and presence transitions, are live-only.
Current latest-value state appears in the snapshot or a targeted ConnectRPC
read when clients need convergence.

### Stronger automation delivery is separate

The realtime API does not promise that a client which is offline beyond the
resume window will receive every intermediate transition. The fallback
snapshot restores current state, but it cannot recreate changes that cancel
each other while the client is offline.

An integration that must eventually process every durable trigger needs a
separate delivery contract, such as acknowledged outgoing webhooks or a paged
public activity API. That future contract can reuse semantic public event
messages. It must not turn every realtime WebSocket into a durable server-side
consumer.

### Protocol 4 replaces earlier development protocols

The public event contract is intentionally incompatible with earlier
development protocols. The implementation uses behavioral protocol version 4
and rejects earlier versions. Chatto is in alpha, so the server does not retain
those compatibility paths.

The protobuf namespace remains `chatto.realtime.v1`. Package `v1` identifies
the experimental public schema namespace; it is not the behavioral protocol
version.

## Consequences

Bot authors receive domain events that describe Chatto activity instead of a
frontend cache protocol. The same event vocabulary works for the bundled
frontend, alternate clients, and future reliable delivery mechanisms.

The bundled frontend keeps the main benefit of ADR-051: one server-scoped
projection can bootstrap and converge without many startup reads. It now has a
clear snapshot path and a semantic event reducer instead of one projection
mutation format for every phase.

Additive protobuf event variants become easier to evolve. A client can ignore
an event type that it does not use because the envelope still gives it the
next safe cursor. Required behavior still needs a protocol-version boundary.

Recent disconnects recover without a complete snapshot. Long, unsafe, or
expensive gaps use bounded current-state recovery. Realtime does not provide
indefinite event history.

The direct-read path performs work in proportion to the bounded EVT gap. The
process-wide live hub continues to make live NATS subscriptions, decoding, and
projection waits scale with Chatto processes instead of connected clients.

The public mapper must cover every durable fact that has public meaning. Tests
must verify semantic mapping, authorization, current-state replay, cursor
handoff, snapshot fallback, and unknown-event handling.

This is a breaking public API change. Its implementation requires generated
client updates, public documentation updates, release-note guidance, and the
`api-breaking-change` label. Older clients and newer servers, and newer clients
and older servers, fail at the protocol handshake instead of exchanging frames
with incompatible meanings.
