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
   user, lightweight state for every room visible to the viewer, the complete
   visible room-group layout, the current finite pending-notification page and
   complete room notification counts, and every active call visible to the
   viewer. DM participant references remain eager because they define the
   conversation label; channel membership and timeline windows are lazy.

“Every room” includes joined DMs with no message yet. The public directory's
conversation-list policy may hide those rows, but the realtime projection and
live authorization capture are exhaustive so the room can receive its first
message and be opened directly.
2. The subscription names timeline rooms already retained by the client. On a
   room's first view, the client sends `hydrate_room` on the same WebSocket and
   receives an atomic room-membership upsert plus the latest 50 timeline events
   as normal projection operations. Later timeline mutations are emitted only
   for retained rooms. Unretained room facts still update lightweight room and
   viewer state; root messages carry a content-free room-activity operation so
   DM ordering, unread, notification, and call state remain current while the
   single resume cursor advances.
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

The browser retains one projection store for every authenticated server for the
lifetime of the tab. Each store owns an explicit `empty`, `hydrating`, `ready`,
or `stale` phase and its own cursor. Socket teardown does not discard either.
Room timeline retention belongs to that projection state: every reconnect and
inactive poll sends its retained room IDs with the cursor.
The client distinguishes desired rooms from confirmed materialised windows. A
room enters the resumable retained set only after its timeline replacement was
applied, so a dropped hydration response is retried instead of being mistaken
for cached state. Desired retention is capped at the wire limit of 64 rooms;
selecting another room evicts the least-recent inactive timeline and replaces
the socket so the server and client retention sets stay aligned.
The URL-active server has the only persistent realtime WebSocket. Inactive
servers periodically open short-lived connections, resume through `caught_up`,
and close; these catch-ups are serialized across the browser and use the same
frames and reducer as the active connection. Polling is therefore a transport
lifecycle, not a second bootstrap mechanism.

Browser wake-up, socket close, online, and heartbeat-stall signals act only on
the transport. They never trigger parallel ConnectRPC refreshes for canonical
server, room, timeline, notification, presence, or active-call state. A tab
that was hidden for at least 30 seconds replaces its active WebSocket and
resumes the retained projection. If its cursor expired while asleep, the
server emits the normal compacted reset on that replacement stream.

Resume cursors are encrypted and authenticated with a purpose-separated key
derived from `core.secret_key`, use a random nonce, and are bound to the
authenticated viewer. EVT stream incarnation and global sequence remain inside
the sealed payload and are never disclosed as public API facts. Tampering,
cross-user reuse, secret rotation, and foreign stream incarnation select a
compacted reset. Room-timeline pagination cursors follow the same confidentiality
and integrity invariant, and bind their sequence boundary to the authenticated
viewer plus the exact room or room/thread-root resource; legacy plaintext
`seq:` cursors and cross-resource reuse are rejected.
Realtime resume cursors carry a sealed issue time and expire after 24 hours.
Expiry selects compacted current state, so clients converge without retaining
an indefinitely reusable replay credential.

The server creates no new JetStream stream and no per-client consumer. For a
valid short gap it captures an EVT cutoff, waits until every registered
projection is current before reading snapshot or authorization state, and
performs bounded point reads by
global stream sequence, and derives public projection operations from the
current read models. It subscribes the connection to the process-wide
`MyEventsHub` first, then discards buffered duplicates through the cutoff before
continuing live. A gap is limited to 10,000 EVT sequences and 2,000 delivered
facts; exceeding either bound selects compacted reset instead.

Replay and compacted bootstrap use the same process-local capacity guard. Each
replica admits at most eight catch-ups globally and one per authenticated user,
with a per-user burst of three stale-cursor replay attempts and one restored
token every 20 seconds. Cursorless compacted bootstraps and current-boundary
reconnects use a separate general catch-up bucket with a burst of 20 and one
token restored each second because neither can request historical events at
classification time. If EVT advances before planning completes, the server
also consumes a replay token before delivery. All classes remain subject to
the concurrency guard. A catch-up has a
30-second whole-operation deadline. Rejected clients
receive explicit reconnect guidance. These controls bound replica work but do
not participate in correctness, authorization, replay position, or any
cross-replica invariant.

Post-catch-up `hydrate_room` work shares the process-wide semaphore and is
serialized per user across that user's sockets. A separate per-user bucket
admits a burst of 20 room hydrations and restores one token per second.
Clients serialize their own outstanding hydration requests. Non-fatal
admission errors identify the rejected room and carry a retry delay so the
request can resume on the same connection rather than waiting for reconnect.
Compacted prefixes are emitted frame by frame instead of retaining a second
frame graph, and the 64-room retention bound limits each reset to at most 3,200
decrypted recent timeline rows.

Projection hydration reuses the public ConnectRPC assemblers. PII is decrypted
only at this authenticated response boundary and only for requested timeline
rooms. Message retractions and account
key shredding are resolved to their current tombstone form, so replay never
re-emits an obsolete plaintext body. Room and RBAC visibility changes either
emit explicit resource removal or force a reset from current authorization.
Membership loss also establishes a persistent client privacy fence: canonical
and mounted timeline reducers reject later rows until an explicit positive
membership operation arrives. The same server event replaces active calls and
notifications from the viewer's new authorization state, and the client
disconnects local call media for the revoked room without writing another
leave intent.
`reset` immediately invalidates every projection-derived frontend mirror before
the multi-frame compacted prefix is applied, so an interrupted reset cannot
leave stale notifications, calls, permissions, preferences, or authenticated
runtime settings visible.
Channel echoes remain projection rows linked to their canonical thread reply.
Reaction changes refresh both visible forms, while disabling an echo emits an
explicit timeline-row removal rather than misrepresenting it as a deleted
message tombstone. Canonical reply deletion marks the corresponding echo
upsert as a retained deleted row so it remains a tombstone rather than taking
the direct-echo deletion path.

Notification records and room/thread read markers include latest-value state
outside EVT. Every subscription therefore re-emits the viewer resource, every
visible room's viewer state, the complete followed-thread viewer-state set,
pending notification page/counts, and directory presence before `caught_up`.
Missing followed-thread entries clear retained follow/unread flags. Transient
signals buffered during the handoff converge concurrent changes. Thread follow
and read-marker mutations share a user-scoped viewer-state invalidation, which
is remapped to the current root row after reconciliation. The authoritative
followed-thread replacement fails the catch-up on an uncertain membership,
metadata, follow, or read-marker read instead of silently omitting that row.
Directory metadata
facts are fanned to sessions when the viewer has
not joined the room. The shared hub caches each projection user's authorized
directory rooms, suppresses facts for rooms the user has never been able to
see, and emits removal after visibility loss only for previously visible rooms.

Presence is likewise latest-value state and cannot be reconstructed from EVT.
The reconciliation includes complete `presences_replace` state from the
process-wide `PresenceHub`; later transitions use the existing transient live
envelope. This keeps the one-stream/one-reducer contract while allowing a
dormant server to converge on activation.

Authenticated server presentation and runtime settings are canonical client
state. They are therefore included in the compacted prefix and replaced by a
projection operation after server updates; the client does not bootstrap or
refresh them through a separate ConnectRPC read. Typing, presence transitions,
attention hints, and session termination remain non-replayable envelopes on the
same WebSocket; presence additionally has the finite convergence operation
described above. Notification create/dismiss signals, viewer preferences,
thread follow/read state, and profile changes instead assemble authoritative
projection operations. A notification replacement may include optional
live-only transition metadata for presentation effects such as sounds; replay
and finite reconciliation omit it. Active call state is canonical and uses
`active_calls_replace` in the compacted prefix and after durable call
transitions. These transient values do not
define the durable client projection and are not replayed. Lazy room hydration
does not create a separate bootstrap/feed path: it is a control request on the
same WebSocket, and its atomic room/timeline replacement is applied by the same
projection reducer as compacted reset, resumed replay, and live delivery.

Version 2 is the sole bundled 0.5.0 client/server contract and is intentionally a
breaking semantic change for clients that previously treated every realtime
frame as a domain-event notification. The bundled 0.5 frontend requires a 0.5
server because a 0.4 server cannot provide its canonical bootstrap projection;
remote frontend/server compatibility CI therefore starts a new patch-series
baseline when the first stable 0.5 release exists.

The transient `RealtimeEventEnvelope` no longer declares durable message,
reaction, room, thread-creation, custom-status, asset, call, notification,
viewer-preference, thread-follow/read, server-layout, or member-removal
alternatives; their former field numbers and names are reserved. Integrators
migrate those handlers to `RealtimeProjectionEvent` operations and retain the
envelope only for non-replayable signals: typing, presence, mention/new-DM
attention hints, and session termination.

`user_remove` purges the directory resource and every copied render reference
to that user in retained membership, timeline includes, notification actors,
active calls, and frontend room/thread mirrors. Historical facts retain stable
actor IDs, but deleted profile PII is not retained for display.

This client projection protocol complements rather than replaces
`chatto.api.v1`. Integrations continue to use the resource-oriented ConnectRPC
services for explicit reads, pagination, commands, and read-your-writes
responses. The projection stream is the ordered bootstrap/convergence surface
for clients that choose to maintain a server-wide local view.

## Consequences

Room switching among retained rooms is a rendering selection over server-owned data. Temporary
historical/permalink windows are discarded when their room is deselected, so
late query responses cannot replace that room's retained latest projection.
Server chrome, sidebar branding, permissions, unread state, and DM labels are
selectors over the same projection; they do not run a second authenticated
bootstrap query when a server becomes visible. The first visit to a cold room
shows its timeline loading state until the stream emits its replacement.
Reactions, edits, retractions, channel-echo additions/removals, and new messages
remain current for retained rooms. A never-viewed room begins from current
aggregate state when hydrated instead of replaying timeline transitions the
client never retained.
Thread timeline stores are reference-counted by mounted panes and disposed
after their final consumer unmounts; reconnect/reset therefore reloads only
open threads, not every thread viewed during the tab session.

Switching among already-hydrated rooms and servers also renders retained state
immediately, then resumes it in the background. Known rooms do not return to a
loading skeleton merely because their transport was dormant. A room absent from
a stale projection is not treated as authoritatively missing until activation
reaches `caught_up`. Cold server and first-room hydration, thread/history windows, non-member
previews, and media remain independently loadable and may still show loading
states.

At most one background catch-up socket exists alongside the active persistent
socket. Inactive polls are jittered and serialized, and a stalled poll is
abandoned after 30 seconds so one server cannot block the rest. Intentional
dormancy is a distinct healthy transport status, not a connection failure.
Non-replayable
typing, presence transitions, and similar transient frames are intentionally
not reconstructed while a server is dormant; activation observes current
latest-value state and later transitions according to their owning subsystem.
Presentation-only browser resume work, such as re-measuring virtualized lists
or advancing local expiry clocks, remains independent of server catch-up and
must not initiate canonical data reads.

Initial connection payload and server hydration work grow with server users and
visible room summaries, not with every joined-room timeline. A first room view
materialises at most 50 events plus complete channel membership. Compacted
resets hydrate only timelines the client already retained, with bounded
concurrent server-side assembly.

Retained canonical timeline windows remain capped at 50 rows per hydrated room.
Replacement operations include opaque cursors for every retained row and live
upserts include the canonical row cursor, so the client advances its oldest
pagination boundary without a separate refresh read.
The frontend creates heavier render/message stores and asks for channel
membership/timeline hydration lazily when a room is first selected; never-viewed
DM histories therefore do not accumulate in the client projection.
Duplicate hydration controls are ignored once a room is retained, preventing
clients from repeatedly forcing timeline assembly and PII decryption.
Ordinary message delivery refreshes only the room's lightweight viewer state
(including unread state), not its notification count, metadata, or complete
membership list. Notification counts converge independently through
notification signals and resume reconciliation. Room selection remains a pure
rendering concern after that room's first hydration, even after live rows have
rolled through the capped window.

The stream is a convergence feed, not an audit log. Replay uses current
authorization, deletion, and erasure state; it may reset rather than reproduce
historical public shapes. Clients must apply operations in order and persist a
cursor only after all preceding operations have succeeded. Integrators offline
for more than 24 hours still converge through compacted state, but cannot
recover every historical transition from the expired interval.

Clients fail closed on an undecodable frame or unknown projection operation.
They validate an entire projection event before mutating state and do not
accept a later cursor across input they did not understand. Completed inactive
polls are immediately marked stale after closing: known retained resources stay
renderable, while missing resources remain non-authoritative until activation
catches up.

No new durable projection or NATS resource is introduced. EVT remains the
source of durable facts, existing read models remain the source of public
resource shapes, and the process-wide hub remains the sole live ingress per
Chatto process.
