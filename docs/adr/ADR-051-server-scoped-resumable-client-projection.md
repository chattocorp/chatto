# ADR-051: Server-Scoped Resumable Client Projection

**Date:** 2026-07-16

## Context

The bundled client previously loaded only the room being rendered. It combined
ConnectRPC reads with live invalidation events and repeated projected reads
after reconnect. That model could recover message rows in the visible room, but
it could not reconstruct reactions on older messages or keep inactive joined
rooms current. Switching rooms was therefore both a navigation and data-loading
operation.

Exposing EVT would couple public clients to internal persistence messages,
encrypted fields, aggregate evolution, and projection timing. Creating a
JetStream consumer per client would make reconnect correctness expensive and
would duplicate the process-wide realtime hub's fanout work. A separate
snapshot API would also give clients two state-ingestion mechanisms whose
ordering and reducers could diverge.

## Decision

Realtime protocol version 2 is a server-scoped projection stream. Its public
unit is an idempotent `RealtimeProjectionOperation`, not an EVT payload. The
same ordered envelope and client reducer handle both initial convergence and
later changes:

1. A fresh client receives `reset`, the current public server profile,
   authenticated server runtime state, viewer resource, every public directory
   user, every room visible to the viewer with membership references into that
   user directory, the complete visible room-group layout, and the latest 50
   timeline events for every room the viewer has joined, the current finite
   pending-notification page and complete room notification counts, and every
   active call visible to the viewer.
2. The client applies later projection operations to those resources and room
   timelines regardless of which room is being rendered.
3. A cursor is issued only at EVT boundaries. A socket reconnect in the same
   in-memory client session supplies its last applied cursor and receives
   projection operations derived from later EVT facts before `caught_up`.
4. A missing, invalid, expired, foreign-incarnation, authorization-sensitive,
   or oversized cursor causes another `reset` plus current compacted state.

The 0.5 bundled client requires the discovery capability
`chatto.realtime.projection.v1` and does not retain the 0.4 ConnectRPC bootstrap
as a fallback. A 0.4 server is therefore an explicit unsupported target for the
0.5 client. The 0.5 server accepts only protocol version 2 and rejects omitted,
version-1, and unknown handshakes with `unsupported_protocol`. The protobuf
package remains `chatto.realtime.v1`; that suffix is an API namespace, not the
accepted behavioural protocol version.

The browser does not persist a cursor independently of its in-memory
projection. Reloading the page or recreating a server store omits the cursor and
therefore rebuilds the complete projection. This prevents a valid cursor from
being applied to an empty client store.

Resume cursors are encrypted and authenticated with a purpose-separated key
derived from `core.secret_key`, use a random nonce, and are bound to the
authenticated viewer. EVT stream incarnation and global sequence remain inside
the sealed payload and are never disclosed as public API facts. Tampering,
cross-user reuse, secret rotation, and foreign stream incarnation select a
compacted reset. Room-timeline pagination cursors follow the same confidentiality
and integrity invariant; legacy plaintext `seq:` cursors are rejected.

The server creates no new JetStream stream and no per-client consumer. For a
valid short gap it captures an EVT cutoff, performs bounded point reads by
global stream sequence, and derives public projection operations from the
current read models. It subscribes the connection to the process-wide
`MyEventsHub` first, then discards buffered duplicates through the cutoff before
continuing live. A gap is limited to 10,000 EVT sequences and 2,000 delivered
facts; exceeding either bound selects compacted reset instead.

Projection hydration reuses the public ConnectRPC assemblers. PII is decrypted
only at this authenticated response boundary. Message retractions and account
key shredding are resolved to their current tombstone form, so replay never
re-emits an obsolete plaintext body. Room and RBAC visibility changes either
emit explicit resource removal or force a reset from current authorization.
Channel echoes remain projection rows linked to their canonical thread reply.
Reaction changes refresh both visible forms, while disabling an echo emits an
explicit timeline-row removal rather than misrepresenting it as a deleted
message tombstone. Canonical reply deletion marks the corresponding echo
upsert as a retained deleted row so it remains a tombstone rather than taking
the direct-echo deletion path.

Notification records live in `RUNTIME_STATE`, not EVT. Their finite current
page and room counts are therefore re-emitted inside the projection stream on
every valid resume before `caught_up`; transient create/dismiss signals buffered
during the handoff then converge any concurrent changes. Directory metadata
facts are fanned to sessions when the viewer has
not joined the room. The shared hub caches each projection user's authorized
directory rooms, suppresses facts for rooms the user has never been able to
see, and emits removal after visibility loss only for previously visible rooms.

Authenticated server presentation and runtime settings are canonical client
state. They are therefore included in the compacted prefix and replaced by a
projection operation after server updates; the client does not bootstrap or
refresh them through a separate ConnectRPC read. Transient latest-value state
such as presence, typing, and notification hints can continue to use existing
live envelopes on the same WebSocket. Active call state is canonical and uses
`active_calls_replace` in the compacted prefix and after durable call
transitions. These transient values do not
define the durable client projection and are not replayed. This does not create
a separate bootstrap/feed path: all canonical server resources and room
timelines converge through projection operations.

Version 2 is the sole bundled 0.5.0 client/server contract and is intentionally a
breaking semantic change for clients that previously treated every realtime
frame as a domain-event notification. The bundled 0.5 frontend requires a 0.5
server because a 0.4 server cannot provide its canonical bootstrap projection;
remote frontend/server compatibility CI therefore starts a new patch-series
baseline when the first stable 0.5 release exists.

The transient `RealtimeEventEnvelope` no longer declares durable message,
reaction, room, thread-creation, custom-status, asset, or call alternatives;
their former field numbers and names are reserved. Integrators migrate those
handlers to `RealtimeProjectionEvent` operations and retain the envelope only
for non-replayable signals such as typing, presence, attention hints,
preferences, and session termination.

## Consequences

Room switching is a rendering selection over server-owned data. Temporary
historical/permalink windows are discarded when their room is deselected, so
late query responses cannot replace that room's retained latest projection.
Reactions, edits, retractions, channel-echo additions/removals, and new
messages received while a room is inactive remain
current, and reconnect can recover exact reaction transitions for integrations
while also transmitting the authoritative aggregate message row.

Initial connection payload and server hydration work grow with server users,
visible rooms, and joined-room timeline windows. Timeline hydration is bounded
to 50 events per joined room and concurrent server-side assembly. If unusually
large servers emerge, room-window hydration may become lazy only if it keeps the
single projection stream and reducer contract.

Retained canonical timeline windows remain capped at 50 rows per joined room.
Replacement operations include opaque cursors for every retained row and live
upserts include the canonical row cursor, so the client advances its oldest
pagination boundary without a separate refresh read.
The frontend creates heavier render/message stores lazily when a room is first
selected; inactive server projection rows do not accumulate without bound.
Ordinary message delivery refreshes only the room's lightweight viewer state
(including unread state), not its notification count, metadata, or complete
membership list. Notification counts converge independently through
notification signals and resume reconciliation. Room selection remains a pure
rendering concern even after live rows have rolled through the capped window.

The stream is a convergence feed, not an audit log. Replay uses current
authorization, deletion, and erasure state; it may reset rather than reproduce
historical public shapes. Clients must apply operations in order and persist a
cursor only after all preceding operations have succeeded.

No new durable projection or NATS resource is introduced. EVT remains the
source of durable facts, existing read models remain the source of public
resource shapes, and the process-wide hub remains the sole live ingress per
Chatto process.
