# Realtime Delivery Inventory

Key files:

- [`proto/chatto/realtime/v1/realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto)
- [`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go)
- [`cli/internal/http_server/realtime_projection.go`](../../cli/internal/http_server/realtime_projection.go)
- [`cli/internal/connectapi/realtime_projection.go`](../../cli/internal/connectapi/realtime_projection.go)
- [`cli/internal/core/my_events_model.go`](../../cli/internal/core/my_events_model.go)
- [`cli/internal/core/realtime_replay.go`](../../cli/internal/core/realtime_replay.go)
- [`apps/frontend/src/lib/state/server/projection.svelte.ts`](../../apps/frontend/src/lib/state/server/projection.svelte.ts)
- [`apps/frontend/src/lib/state/server/eventBus.svelte.ts`](../../apps/frontend/src/lib/state/server/eventBus.svelte.ts)
- [`apps/frontend/src/lib/state/server/realtimeSync.svelte.ts`](../../apps/frontend/src/lib/state/server/realtimeSync.svelte.ts)
- [`apps/frontend/src/lib/presenceTracking.ts`](../../apps/frontend/src/lib/presenceTracking.ts)

Related decisions: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md) and [ADR-051](../adr/ADR-051-server-scoped-resumable-client-projection.md).

The protobuf realtime API is mounted at `GET /api/realtime` and upgrades to a
binary WebSocket. The first client frame must be `hello`; the server accepts
only protocol version 2 and authenticates either the hello bearer token or an
existing cookie session. The second client frame must be `subscribe_events`.
It may name room timelines already retained with the projection. After
subscription, `hydrate_room` materialises another joined room over the same
ordered stream.

The `chatto.realtime.v1` package name is the protobuf namespace, not the
behavioural protocol version. Protocol 2 is the server-scoped projection
stream. It uses `RealtimeProjectionEvent`, an optional resume cursor on
`subscribe_events`, and `caught_up` at the replay-to-live boundary. Application
heartbeats and client `ping`/server `pong` share the same connection.

The bundled client creates its event-bus reducer before discovery completes so
consumers can register synchronously, but it opens the WebSocket only after
discovery advertises `chatto.realtime.projection.v1`. Servers older than 0.5 do
not advertise that required contract and are reported as unsupported rather
than receiving the former ConnectRPC bootstrap plus protocol-v1 live feed. An
`unsupported_protocol` error is terminal for the current bus and does not enter
the reconnect loop.

The browser keeps the event bus, projection, readiness phase, and opaque cursor
for every authenticated server in memory for the tab session. Transport is
separate: the URL-active server is `live`, inactive servers are normally
`dormant`, and one inactive server at a time may be `polling`. A poll opens the
same `/api/realtime` stream with that projection's cursor and closes as soon as
`caught_up` arrives. Initial inactive hydration runs immediately; later polls
run about once a minute with jitter and a 30-second client timeout. Switching
servers closes the previous persistent socket without discarding its state and
promotes the selected server to the sole persistent connection.

The frontend keeps an authenticated server's realtime stream connected
independently of the local presence mode. "Look offline" stops presence
refreshes and lets the live presence record expire; it does not pause event
delivery. Realtime connection establishment itself does not touch presence.

Returning to a tab after at least 30 seconds hidden replaces the active
transport even when the browser still reports its old WebSocket as open. The
replacement supplies the retained projection cursor and room set. Browser
visibility, `pageshow`, online, socket-close, and heartbeat signals do not
start parallel ConnectRPC refreshes for canonical projection data. They only
restore transport liveness; replay or a compacted reset performs convergence.

## Compacted projection prefix

A subscription without a usable cursor emits one ordered stream of
idempotent operations:

- `reset`;
- current public server profile, authenticated server presentation/runtime
  state, and authenticated viewer state;
- every public server directory user;
- lightweight state for every room visible to the viewer and the complete
  visible room-group layout; DM participant references remain eager;
- complete channel membership and the latest 50 renderable timeline events only
  for rooms named as retained by the subscribing client;
- the newest finite pending-notification page and complete per-room counts;
- every active call visible to the viewer; and
- a complete latest-value presence map for the projected user directory.

The snapshot builder uses the same ConnectRPC assemblers as public reads. It
decrypts PII only at the authenticated response boundary and resolves messages
through current deletion and key-shredding projections. Deleted or
crypto-erased bodies therefore appear only as normal tombstones. Requested
timeline windows are assembled concurrently with bounded concurrency.
Never-viewed room bodies are not decrypted during bootstrap.

The projection's room set is exhaustive rather than sidebar-policy-filtered:
it includes joined DMs that do not yet contain a message. Public directory
lists may continue hiding those empty conversations, but the projection and
live-hub authorization capture retain them so a `StartDM` response can be
navigated immediately and its first message can arrive through the stream.

The frontend applies this prefix and every later event through the same
`ServerProjectionStore` reducer. Server profile, MOTD, and runtime capability
changes replace canonical projection state instead of causing a ConnectRPC
refresh. Canonical timeline pages evict rows beyond their newest 50. Heavier
message stores are created lazily, and selecting a cold room sends
`hydrate_room`. The response atomically replaces its full room membership and
current timeline through the normal projection reducer; it is not a ConnectRPC
bootstrap.

Timeline replacements carry an opaque cursor for every retained row, and later
row upserts carry that row's cursor. The reducer can therefore advance its
pagination boundary using only the projection stream. Each timeline cursor is
encrypted, authenticated, and bound to its viewer plus exact room or
room/thread-root resource, so it cannot be reused as another timeline's
boundary.

On `reset`, the frontend immediately clears the canonical projection and all
projection-derived mirrors, including cached profiles, notifications, calls,
preferences, permissions, and authenticated runtime settings. Later snapshot
operations repopulate those stores through the normal reducer.

Changing the route selects retained state immediately after a room's first
hydration. A cold route briefly renders its timeline loading state while the
same WebSocket materialises it. DM labels resolve eager participant references,
while selected channel-member lists resolve hydrated membership through the
already-warm user projection. Server chrome and gutter entries likewise select
projected branding, viewer capabilities, notification preferences, and unread
state instead of independently fetching server/viewer/room snapshots.

The room Files sidebar remains a separate, server-scoped lazy cache rather than
part of the compacted realtime prefix. Each room starts with an empty cache and
performs its attachment-list read only when Files is first opened. Later
attachment-relevant timeline message upserts reconcile attachment rows in
hydrated caches. Updates racing the first read are queued and applied to its
result, while updates racing pagination fence the stale page response.
Projection-only timeline-row removals do not remove the underlying message's
files. Reset and room-access loss clear the cache with the other
content-bearing mirrors; a reset rehydrates it when Files remains visible.

Projection readiness distinguishes cold data from transport freshness. Known
rooms in `ready` or `stale` projections render immediately, including after a
server switch. Absence in a stale projection is not authoritative until the
activation catch-up reaches `caught_up`. Loading placeholders remain for a cold
projection, a room's first timeline hydration, and separately lazy history,
threads, previews, and media.

## Resume and live handoff

The sealed cursor contains an EVT stream incarnation, global sequence, and
viewer binding. XChaCha20-Poly1305 protects it with a purpose-separated key
derived from `core.secret_key`; random nonces prevent equal payloads producing
equal tokens. NATS and JetStream coordinates are never public API facts.
Tampering, cross-user reuse, secret rotation, or foreign stream incarnation
selects a compacted reset. Every cursor also carries a sealed issue time and
expires after 24 hours; expiry selects the same safe reset, limiting captured
cursor reuse while still allowing ordinary reconnect gaps. The browser retains
a cursor only with its corresponding in-memory projection. Socket
reconnects can resume; page reloads and recreated stores omit it and receive a
new compacted prefix. A tab waking after more than 24 hours still presents its
expired cursor, and the server responds with the same compacted reset used for
any other unusable cursor. The client clears and rebuilds the retained
projection through normal operations, then marks it ready only at `caught_up`.

For a valid short gap, the handler subscribes to the process-wide live hub,
captures an EVT cutoff, waits until every registered projection is current
before reading authorization or compacted state, and performs bounded
JetStream point reads for the
sequences after the cursor. It does not create a JetStream consumer. Each
deliverable room, asset, or user fact waits for its owning projection and is
converted to current public resource operations. The handler sends `caught_up`
at the cutoff, discards buffered live duplicates through that sequence, and
continues with the hub stream.

The connection retains only a set of hydrated room IDs. Projection mapping
omits room-timeline assembly for every other room, avoiding message-body
decryption and transfer. Recognized durable facts that have no remaining
operation are still emitted as empty projection envelopes with their sealed
cursor, so one global resume position can advance without making unhydrated
timeline history part of client state. On reconnect the client resends retained
IDs; a compacted reset includes only those room windows.

Effective membership changes are authoritative timeline boundaries. When a
universal room stops granting membership, live mapping pairs its current room
state with an empty replacement for any retained timeline plus authoritative
active-call and notification replacements; loss of room
visibility uses `room_remove`, which has the same eviction effect. The browser
also scrubs canonical rows, mounted room stores, open thread stores, optimistic
state, call and notification mirrors, and in-flight reads as soon as projected
membership becomes false. It also disconnects local call media for that room
without issuing a redundant leave command. The privacy fence stays closed until an explicit
positive membership operation arrives, so delayed pagination, previews,
read-your-writes responses, and timeline replacements cannot restore plaintext.

The browser keeps only the non-plaintext retained-room intent. If membership
later returns, the server rematerialises the current window only for that
retained room; never-requested rooms remain lazy. A disconnected client whose
gap contains an authorization-sensitive revocation receives a compacted reset
instead of incremental replay.

The browser advertises a room as retained only after applying its timeline
replacement. Desired rooms with lost or unavailable hydration responses remain
pending and are requested again on the next socket. The browser sends one lazy
hydration at a time; a non-fatal capacity or rate rejection identifies the room
and supplies a retry delay, after which the browser resends it on the same
socket. Both client and server cap retention at 64 room IDs, and the server
ignores duplicate hydration work.
At the bound, the browser evicts its least-recent inactive timeline and replaces
the socket before materialising the newly selected room.

Post-catch-up room hydration shares the process-wide catch-up semaphore and is
serialized per authenticated user across all of that user's sockets. Its token
bucket permits a burst of 20 hydrations and restores one token per second. A
compacted reset emits frames incrementally and materialises at most 64 retained
windows (3,200 recent rows), bounding decryption and transient response memory.

Every subscription emits one finite latest-value reconciliation before
`caught_up`. It replaces the viewer resource; every visible room's read and
permission state; the complete followed-thread viewer-state set, including
RUNTIME_STATE unread markers; pending notifications and room counts; and the
server directory's current presence. Missing followed-thread entries
authoritatively clear follow/unread state on retained thread roots. Buffered
live signals cover mutations concurrent with this reconciliation. Thread
follow/unfollow and read-marker advances publish the same user-scoped
viewer-state invalidation; after the finite replacement, a buffered signal is
mapped to the current root timeline row. The complete followed-thread reader
returns an error for uncertain membership, room metadata, follow, or read-marker
state, so catch-up retries rather than converging to a lossy replacement.

This operation set closes the parts of client state that an EVT gap alone
cannot reconstruct, without a ConnectRPC side read or a second bootstrap
mechanism. Presence and later room/thread read transitions use buffered live
signals on this same stream; durable config changes that affect viewer permissions or
preferences select a compacted reset through their EVT subjects.

Replay scans at most 10,000 EVT sequences and emits at most 2,000 durable
facts. Missing, malformed, expired, foreign-incarnation, oversized, or
authorization-sensitive gaps select the compacted prefix instead of failing
the subscription.

Incremental replay and compacted bootstrap share one process-local catch-up
admission guard. Each replica admits at most eight catch-ups at once and one at
a time per authenticated user. Explicit stale-cursor replay attempts use a
per-user token bucket with a burst of three and one token restored every 20
seconds. Cursorless compacted bootstraps cannot request historical events, and
current-boundary reconnects have no gap, so both use a separate general catch-up
bucket with a burst of 20 and one token restored each second. If EVT advances
between boundary classification and replay planning, the server charges a
replay token before emitting any replay frames, in addition to its general
token. Every admitted catch-up
has a 30-second whole-operation deadline. Capacity rejection sends
`catch_up_in_progress`, `catch_up_rate_limited`, or `catch_up_server_busy` with
reconnect guidance; deadline exhaustion sends `catch_up_timeout`. These limits
bound work and protect availability only. They are deliberately process-local,
and no correctness or authorization decision depends on them.

The metrics endpoint exposes active and total admitted catch-ups, timeouts, and
capacity rejections through `chatto_realtime_catch_ups`,
`chatto_realtime_catch_ups_started_total`,
`chatto_realtime_catch_ups_timed_out_total`, and
`chatto_realtime_catch_ups_rejected_total`.

Reaction facts produce a timeline-event upsert containing the current
aggregate reaction state and a `reaction_change` describing the exact actor,
emoji, and add/remove transition. Message edits, retractions, and reactions
hydrate the canonical current message row rather than exposing internal EVT.
When a thread reply has a visible channel echo, reaction facts upsert both the
canonical reply and its echo row. A direct retraction that disables only the
echo emits `room_timeline_event_remove`; ordinary deleted messages remain
renderable tombstone upserts.

RBAC facts are fanned through the shared hub. The mapper responds with a
reconnecting `projection_reset_required` close so the next subscription starts
from current authorization.

## Process-wide live ingress

`MyEventsHub` owns one NATS Core subscription to `live.sync.>` and one to
`live.evt.>` per Chatto process. It classifies subjects before decoding, waits
for projections once, and fans immutable decoded events into count- and
byte-bounded session queues. Sessions for one user share room-visibility state.
There are no per-client NATS or JetStream consumers.
Directory metadata facts for visible nonmember rooms are additionally fanned
to sessions. The hub maintains a per-user cache of
currently authorized directory rooms: facts for a room never seen by that user
are suppressed, while loss of visibility emits removal only when the room was
previously visible.
Directory visibility reads use bounded concurrency outside the hub mutex and
hydrate only room existence, archive state, and visibility permissions.
Administrative membership facts replace the complete current member-reference
list for existing viewers.

Message facts carry lightweight replacements of the affected room summary and
viewer state alongside timeline mutations. Root messages also carry a
content-free `room_activity` operation, allowing unretained DMs to reorder
without exposing or materialising their message. Notification counts converge
through notification signals and the finite resume replacement. Message
delivery does not reassemble or retransmit complete channel membership. Echo
tombstone upserts explicitly distinguish
canonical-reply deletion from direct echo removal.

Room-read signals emit a `RoomViewerStateReplace` for the affected room and a
finite `NotificationsReplace`. This keeps the retained canonical room row,
pending-notification state, and both sidebar indicators in step, so a later
mutation cannot restore stale unread or mention state. Root-message activity
operations advance the affected room even when its timeline is not retained;
later viewer-state replacements therefore cannot undo DM sorting.

A durable projection hydration or mapping failure closes the session
without advancing its cursor. Reconnect retries that EVT sequence or selects a
compacted reset, so a later cursor cannot make a dropped mutation permanent.
Historical message creation for an echo that is hidden in current projection
state maps to an idempotent timeline removal. Asset processing and deletion
facts map to authoritative upserts of their owning message and any visible
channel echo, so replay never advances beyond a durable attachment mutation
without applying its current render state.

The browser applies the same fail-closed rule. An undecodable frame or unknown
projection operation closes the socket, leaves the preceding cursor intact,
and retries from that position. A projection event is validated in full before
either reducer mutates state, preventing partial application of an atomic
event. A completed inactive poll becomes `stale` as soon as its socket closes:
known resources remain renderable, but absence is not authoritative while the
transport is dormant.

Mounted room stores may retain deliberately paginated history. Thread stores
are reference-counted by mounted thread panes and disposed after their final
consumer unmounts, so inactive threads receive no later fanout and are not
reloaded during reset.

Typing, presence transitions, mention/new-DM attention hints, and session
termination continue as `RealtimeEventEnvelope` frames on the same WebSocket.
Notification create/dismiss signals instead assemble an authoritative
`notifications_replace`; a live replacement may carry transition metadata for
one-shot presentation effects, while replay and finite reconciliation omit it.
Viewer preferences, thread follow/read state, profile changes, server layout,
and member removal likewise mutate the client only through projection
operations. Active calls converge through `active_calls_replace` in the
compacted prefix and after every durable call transition. Transient frames have
no durable cursor; finite pending-notification and presence state are
reconciled explicitly on every subscription. The process-wide PresenceHub
retains current presence and fans out later transitions.

A `user_remove` operation purges copied profile fields from room membership,
timeline includes, notification actors, active-call participants, retained
message/thread render stores, and the shared profile cache. Historical rows may
retain the stable user ID, but not a renderable user object.

Process-wide ingress loss or projection-readiness failure quarantines the hub
and closes every session. A slow session that exceeds its queue limits is
closed independently. Both cases reconnect through resume or a compacted reset
rather than continuing a healthy-looking stream across an unobservable gap.

WebSocket connections use small read/write buffers and share a write-buffer
pool. When compression is enabled, the server uses Huffman-only DEFLATE and
compresses frames of at least 1 KiB.

| Endpoint        | Frame schema                                          | Authorization                                                                                                               | Description                                                       |
| --------------- | ----------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| `/api/realtime` | `chatto.realtime.v1.Realtime*` binary protobuf frames | Bearer token in hello or cookie auth; current per-resource and room visibility is applied before public projection mapping. | Protocol 2 server-scoped compacted/resumable projection delivery. |

The realtime client projection does not supersede `chatto.api.v1`. Public
ConnectRPC resources remain the integrations surface for explicit reads,
pagination, mutations, and read-your-writes responses; realtime protocol 2 is
an optional ordered convergence feed for clients maintaining local state.
