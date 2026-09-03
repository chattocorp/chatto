# Realtime Delivery Inventory

Key files:

- [`realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto)
- [`events.proto`](../../proto/chatto/realtime/v1/events.proto)
- [`room_group_events.proto`](../../proto/chatto/realtime/v1/room_group_events.proto)
- [`transient_events.proto`](../../proto/chatto/realtime/v1/transient_events.proto)
- [`realtime.go`](../../cli/internal/http_server/realtime.go)
- [`realtime_consistency.go`](../../cli/internal/connectapi/realtime_consistency.go)
- [`eventBus.svelte.ts`](../../apps/frontend/src/lib/state/server/eventBus.svelte.ts)
- [`realtimeResources.ts`](../../apps/frontend/src/lib/api-client/realtimeResources.ts)

Related decisions: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md),
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md),
[ADR-090](../adr/ADR-090-semantic-realtime-events-with-bounded-resume.md),
[ADR-091](../adr/ADR-091-use-one-event-vocabulary-for-storage-live-and-realtime.md),
and [ADR-092](../adr/ADR-092-use-a-public-realtime-event-union.md).

## Public protocol

The public API is a binary protobuf WebSocket at `GET /api/realtime`. The
server accepts behavioral protocol version 4. The `chatto.realtime.v1` suffix
is the protobuf package name. It is not the behavioral protocol version.

The first client frame is `hello`. It contains protocol version 4 and can
contain a bearer credential. A same-origin browser can use its cookie session.
The server replies with `hello` and reports the accepted version, server
version, and heartbeat interval. The protocol does not have a capability
matrix.

The second client frame is `subscribe_events`. It contains an optional opaque
resume cursor and a required `SNAPSHOT` or `LIVE_ONLY` fallback choice. The
server replies with `subscribed` and selects one recovery mode:

- `SNAPSHOT`: send an exact authorized content snapshot;
- `LIVE_ONLY`: start at the current boundary without current state or old
  events; or
- `RESUME`: send authorized durable events after the supplied cursor.

The server sends the selected snapshot or replay without another client
request. It then sends `caught_up` with the handoff cursor. The client can
consider the subscription current only after it applies all earlier frames
and this marker.

Client control frames after subscription contain application-level `ping`
frames. The server returns the ping nonce in `pong`.

## Public events

`chatto.core.evt.v1.Event` is the semantic unit for durable facts and
transient signals inside the server. `chatto.realtime.v1.RealtimeEvent` is the
authorized public event shape. It contains common metadata, one public payload
variant, and an optional opaque resume cursor. It does not contain resource
state.

A public event has a stable event ID, source time, visible actor ID, and one
event variant. Variants cover messages, reactions, pins, assets, rooms,
membership, threads, users, calls, and public invalidations. Typing, presence
changes, and session termination use the same public union but have no resume
cursor.

Common metadata and the cursor are outside the event `oneof`. A client can
ignore a new event variant and still retain its cursor after it accepts the
complete frame.

The `RealtimeEvent.event` union and the realtime event files are the public catalogue.
Each union member has the same name and field number as its canonical event.
Public payload field numbers are independent from EVT. A missing union member keeps an internal
variant out of the public API. A missing public payload field keeps an internal
field out of the generated client types.

After event authorization, an exhaustive typed mapper copies approved values
into a new dedicated public payload. It then adds trusted decrypted values to
public-only `_plaintext` fields. Public events do not
expose raw EVT bytes, ciphertext, nonces, storage pointers, private moderation
data, subjects, stream identities, or sequence numbers.

Event authorization must make every delivered field safe for that viewer. A
future field with narrower visibility needs an explicit viewer-aware mapping
rule or a separate authorized shape.

An authorized message-post event carries `body_plaintext` for immediate
display. EVT does not store this field. The frontend inserts a temporary
timeline row from the event ID, actor, time, reply references, and plaintext
body. Values that belong only to the complete message resource start empty.
These values include attachments, link previews, reactions, pin state, thread
counts, thread participants, and the timeline cursor. A background `GetMessage`
read uses the event cursor as its minimum boundary and replaces the temporary
row with the authoritative resource. A wider cursor-bounded timeline-window
refresh then reconciles ordering and pagination cursors. The client does not
save the event cursor if either read fails.

## Exact snapshot and targeted resource reads

`ServerContentView` supplies one exact EVT boundary `E`. The server captures
the complete visible room directory, room-group layout, active calls, public
server profile, and users that these resources reference while the view is at
that boundary. It releases the read barrier before protobuf encoding or
WebSocket writes.

Each `snapshot` frame contains one canonical `chatto.api.v1` resource shape.
The resource families are:

| Resource | Protobuf value | Client meaning |
| --- | --- | --- |
| Server profile | `ServerPublicProfile` | Public server profile at `E` |
| Rooms | `ListRoomsResponse` | Complete visible room directory at `E` |
| Room groups | `ListRoomGroupsResponse` | Complete visible room-group layout at `E` |
| Users | `BatchGetUsersResponse` | Only the viewer and users referenced by visible snapshot resources |
| Active calls | `ListActiveCallsResponse` | Complete visible active-call state at `E` |

The snapshot does not contain the complete user directory. It also excludes
message and thread timelines, search results, files, pins, and other large or
paginated resources. The client reads those resources through ConnectRPC when
it needs them.

Notifications, presence, read markers, account-security state, and process
runtime configuration do not use the EVT boundary owned by
`ServerContentView`. The bundled frontend reads its required auxiliary state
through ConnectRPC before it accepts `caught_up`. These reads do not redefine
the EVT snapshot boundary.

After a durable event, a targeted ConnectRPC request can set
`Chatto-Realtime-Minimum-Cursor` to that event's resume cursor. The common API
interceptor validates the viewer-bound token and waits until the serving
replica includes at least that content boundary. The handler then returns its
normal canonical response.

Room and thread timelines are not unconditional bootstrap families. The
frontend reloads each mounted timeline at `E` through `RoomService` or
`ThreadService`. A read caused by a later durable event uses that event's cursor
as its minimum boundary. Files, pins, search, and other large or lazy data keep
their independent ConnectRPC reads. They are not part of the retained realtime
projection or its cursor. Canonical events act as refresh hints for resources
that the client already uses.

The bundled frontend gives each cursor-bounded ConnectRPC call a 10-second
deadline. A timeout fails reconciliation and closes the socket without cursor
advance. A new resource reset also starts a new local projection generation.
Late bootstrap, user, resource, and timeline responses from an older generation
cannot change the newer projection.

## Bounded resume

The resume cursor is a signed, viewer-bound JWT. Its public claims are `sub`,
`aud`, `p`, `iat`, `exp`, and `v`. The `p` claim is an HMAC of the stream
incarnation, viewer, subscription scope, and EVT sequence. The server recovers
the sequence by comparing at most 10,000 candidates in the retained replay
window. The token expires after 15 minutes. NATS and JetStream coordinates are
never public API data.

Snapshot and resume use this handoff:

1. Subscribe the connection to the process-wide live hub.
2. Validate the optional cursor and capture a stable EVT boundary `E`.
3. Send either an exact snapshot at `E` or authorized durable events through
   `E`.
4. Apply current authorization and map each replayed canonical event to the
   public union.
5. Send `caught_up(E)`, discard buffered durable duplicates through `E`, and
   continue with live delivery.

The direct-read path creates no JetStream consumer. It scans at most 10,000 EVT
sequences and emits at most 2,000 durable events. The complete catch-up has a
30-second deadline. These are independent safety caps. The sequence cap bounds
work even when most events are not visible to the viewer. The emitted-event cap
bounds reducer and transport fanout after authorization. The time limit bounds
the complete operation. The current values are conservative defaults, not
capacity claims. Production measurements can change them without changing the
protocol or cursor shape.

A missing, invalid, expired, foreign-stream, oversized, or
authorization-unsafe cursor selects the requested fallback. A `SNAPSHOT`
client receives a new current-state snapshot. A `LIVE_ONLY` client starts at
`E` and receives no old events. The
server never sends a partial replay and then silently skips to live delivery.

Incremental replay and fallback share one process-local admission guard. Each
replica admits at most eight catch-ups at once and one at a time for each user.
Stale-cursor replay has a per-user burst of three and restores one token every
20 seconds. Cursorless and current-boundary catch-ups use the general burst of
20 and restore one token each second. Metrics expose active, started,
timed-out, and rejected catch-ups.

## Authorization and projection readiness

For a valid short gap, the handler subscribes to the process-wide live hub,
captures an EVT cutoff, waits until `ServerContentView` reaches that cutoff
before it reads membership, applicable message-read permissions, interaction
relationships, or compacted state, and performs bounded JetStream point reads
for the sequences after the cursor. It does not create a JetStream consumer. Each
deliverable room, asset, or user fact uses that same content-view readiness
boundary and is converted to a fresh authorized public event. The handler
sends `caught_up` at the cutoff, discards buffered live duplicates through
that sequence, and continues with the hub stream.

Message and asset events require room membership. A channel viewer also needs
`message.read`, or `message.read-interactions` with a relationship to the
canonical thread root. DM membership authorizes the read. Typing follows the
same message-read boundary.

Room visibility and administrative membership facts update the process-wide
visibility cache. Its stable admission boundary includes room creation,
deletion, Universal changes, joins, leaves, member additions, member removals,
bans, and unbans. Facts for a room that a caller never saw are suppressed.

RBAC facts can revoke access without a public room fact. The server closes
affected connections with `projection_reset_required`. The next subscription
uses current authorization and normally receives a snapshot. A replay can
send a viewer's own leave, removal, or ban fact even when current membership
is false. This closing fact removes state that the client could have retained.

A durable mapping or resource-reconciliation failure closes the connection
before the cursor advances. Reconnect retries the fact or uses a safe fallback.
Unknown public event variants are additive and can be ignored while the
transport cursor advances.

An EVT fact with an unknown aggregate namespace requires a reset because the
replica cannot determine its effect on snapshot state. A user fact also
requires a reset when its subject aggregate ID and payload user ID do not
match. Live delivery closes the connection, and replay selects the requested
safe fallback. Neither path advances the cursor past the fact.

## Process-wide live ingress

`MyEventsHub` owns one NATS Core subscription to `live.sync.>` and one to
`live.evt.>` per Chatto process. It classifies subjects before decoding, waits
for `ServerContentView` once for content facts, and fans immutable decoded
events into count- and byte-bounded session queues. Sessions for one user
share room-visibility state. There are no per-client NATS or JetStream
consumers.

Transient `live.sync.>` messages and durable `live.evt.>` messages use
`chatto.core.evt.v1.Event`. Transient variants use oneof tags 20000 through
29999. The transient wire contains no second envelope or compatibility tag.
During a rolling replacement from an older envelope, old and new replicas
drop each other's transient signals. This is safe because these signals have
no replay contract. Durable facts continue through `live.evt.>`, and a
reconnect snapshot restores current content state.

A NATS continuity gap or projection-readiness failure quarantines the hub and
closes current sessions. The replica admits a new hub generation only after
NATS resources, projections, and volatile watchers are current. A slow session
that exceeds its queue count or byte limit closes independently.

A durable room-group, room-layout, or non-MOTD server-configuration fact does
not yet have a safe public event projection. The hub also quarantines current
sessions for these facts. The next exact snapshot provides current content.
This fail-closed path prevents a transient resource invalidation from racing a
lagging replica. A later public catalogue entry can replace the reset.

## Bundled frontend

The bundled frontend selects `SNAPSHOT`. It resets its server projection when
`subscribed.recovery_mode` is `SNAPSHOT` and applies each snapshot resource.
It saves the `caught_up` cursor only after the snapshot, auxiliary reads, lazy
timeline refreshes, and all earlier event-triggered resource reads succeed.
If the socket closes during a snapshot, the client has no resume cursor and
requests a new snapshot.

The projection stores canonical public resources. It does not store
realtime-specific resource copies. Resource invalidation events start
coalesced ConnectRPC reads. If another event reaches the same resource family
during a read, the frontend runs one follow-up read at the newest event cursor.
Timeline-derived stores refresh their current ConnectRPC windows. A failed
refresh closes the socket without saving that event cursor.

The browser keeps one in-memory resource view and cursor for each
authenticated server. Only the active server keeps a persistent socket.
Inactive servers use bounded periodic catch-up sockets. A page reload starts
without a cursor and performs new resource reads.

The frontend keeps its resource view during access-token rotation, cookie-session
renewal, server switches, network reconnects, and tab wake. It replaces the
socket and sends the same cursor. Human bearer credentials close at
access-token expiry. Cookie connections close at the renewal boundary. The
server revalidates the accepted credential before subscription and once per
minute.

The browser resets its liveness timer on every server frame. A heartbeat can
carry a fresh cursor for the last durable sequence that this socket has
delivered. The client retains it only after earlier reconciliation succeeds. It replaces a
socket after a heartbeat stall. An undecodable or unknown top-level frame
causes a reconnect without cursor advancement. WebSocket connections use small
buffers and a shared write-buffer pool. When compression is enabled, the
server uses Huffman-only DEFLATE for frames of at least 1 KiB.

## Interface boundary

| Endpoint | Frame schema | Authorization | Description |
| --- | --- | --- | --- |
| `/api/realtime` | `chatto.realtime.v1.Realtime*` binary protobuf frames | Bearer credential in `hello` or a same-origin cookie; current resource and room visibility apply before mapping | Protocol 4 exact snapshots, authorized public events, and 15-minute bounded resume |

Realtime does not replace `chatto.api.v1`. ConnectRPC remains the public API
for commands, explicit resource reads, pagination, history, search, and
read-your-writes responses.
