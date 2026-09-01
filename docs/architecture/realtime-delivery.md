# Realtime Delivery Inventory

Key files:

- [`realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto)
- [`server_snapshot.proto`](../../proto/chatto/api/v1/server_snapshot.proto)
- [`realtime.go`](../../cli/internal/http_server/realtime.go)
- [`server_snapshot.go`](../../cli/internal/connectapi/server_snapshot.go)
- [`eventBus.svelte.ts`](../../apps/frontend/src/lib/state/server/eventBus.svelte.ts)

Related decisions: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md),
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md),
[ADR-087](../adr/ADR-087-semantic-realtime-events-with-bounded-resume.md),
and [ADR-088](../adr/ADR-088-use-one-event-vocabulary-for-storage-live-and-realtime.md).

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
resume cursor and a required `SNAPSHOT` or `LIVE_ONLY` initial-state choice.
The server replies with `subscribed` and selects one recovery mode:

- `SNAPSHOT`: send current authorized resource chunks;
- `LIVE_ONLY`: start at the current boundary without resource chunks or old
  events; or
- `RESUME`: send current authorized resource chunks and then send authorized
  durable events after the cursor.

The server sends `caught_up` with the live handoff cursor after the selected
recovery phase. The client can consider the subscription current only after
this frame.

Client control frames after subscription contain application-level `ping`
only. The server returns the nonce in `pong`. Room hydration and retained-room
controls are not part of protocol 4.

## Canonical events

`chatto.core.evt.v1.Event` is the semantic unit for durable facts and
transient signals. `RealtimeEvent` is a transport wrapper that contains one
authorized Event and an optional opaque resume cursor. It does not contain
resource state.

A durable Event has a stable event ID, source time, visible actor ID, and one
event variant. Variants cover messages, reactions, pins, assets, rooms,
membership, threads, users, calls, and public invalidations. Typing, presence
changes, and session termination use the same envelope but have no resume
cursor.

Common metadata is outside the event `oneof`, and the cursor is outside the
canonical Event. A client can ignore a new event variant and still retain its
cursor after it accepts the complete frame.

The server creates a fresh authorized Event for delivery. A public catalogue
omits internal variants. Protobuf field-surface options allow shared,
storage-only, and client-only fields. Unspecified payload fields are denied by
default. The copier does not retain unknown fields. Authorized delivery-only
decrypted values use `_plaintext` fields. Public events do not expose raw EVT
bytes, ciphertext, subjects, stream identities, or sequence numbers.

## Snapshot resources

A snapshot frame contains one `chatto.api.v1.ServerSnapshotChunk`. Each chunk
reuses a canonical public protobuf:

| Resource case | Canonical public protobuf | Client meaning |
| --- | --- | --- |
| `server` | `ServerPublicProfile` | Current public server profile |
| `motd` | `GetMotdResponse` | Current authenticated message of the day |
| `runtime_config` | `GetRuntimeConfigResponse` | Current authenticated runtime configuration |
| `viewer` | `GetViewerResponse` | Current viewer identity and capabilities |
| `users` | `ListUsersResponse` | Complete visible user directory |
| `rooms` | `ListRoomsResponse` | Complete visible room directory |
| `room_groups` | `ListRoomGroupsResponse` | Complete visible room-group layout |
| `notifications` | `ListNotificationOccurrencesResponse` | Bounded occurrence page and complete counts |
| `active_calls` | `ListActiveCallsResponse` | Complete visible active-call state |

A list chunk replaces that complete client resource family. The room directory
includes empty DMs that the caller can access. Each DM resource contains its
complete member IDs and states whether it has message history. Channel
resources leave these DM fields empty.

Snapshot chunks are current resources. They are not synthetic events and do
not have event cursors. The server assembles them with the same authorization
and public resource code as ConnectRPC reads. Deleted or crypto-shredded
plaintext does not return during recovery.

Room and thread timelines are not snapshot resources. The frontend reads
timeline pages through `RoomService` and `ThreadService`. Files, pins, search,
and other large or lazy data also keep their explicit ConnectRPC reads.
Canonical events act as refresh hints for loaded resources.

## Bounded resume

The resume cursor is encrypted, authenticated, and bound to the viewer. It
contains an EVT stream identity, sequence, and issue time inside the sealed
value. It expires after 24 hours. NATS and JetStream coordinates are never
public API data.

Resume uses this handoff:

1. Subscribe the connection to the process-wide live hub.
2. Capture a stable EVT boundary.
3. Wait for registered projections through that boundary.
4. Read the bounded EVT sequence range with JetStream point reads.
5. Send current authorized snapshot resources.
6. Apply current authorization and censor each replayed canonical event.
7. Send `caught_up`, discard buffered duplicates through the boundary, and
   continue with live delivery.

The direct-read path creates no JetStream consumer. It scans at most 10,000 EVT
sequences and emits at most 2,000 durable events. The complete catch-up has a
30-second deadline.

A missing, invalid, expired, foreign-stream, oversized, or
authorization-unsafe cursor selects the requested fallback. A `SNAPSHOT`
client receives current resources. A `LIVE_ONLY` client starts at the current
boundary and receives no old events. The server never sends a partial replay
and then silently skips to live delivery.

Incremental replay and fallback share one process-local admission guard. Each
replica admits at most eight catch-ups at once and one at a time for each user.
Stale-cursor replay has a per-user burst of three and restores one token every
20 seconds. Cursorless and current-boundary catch-ups use the general burst of
20 and restore one token each second. Metrics expose active, started,
timed-out, and rejected catch-ups.

## Authorization and projection readiness

Live delivery, resume, and snapshots use current authorization. Message and
asset events require room membership. A channel viewer also needs
`message.read`, or `message.read-interactions` with a relationship to the
canonical thread root. DM membership authorizes the read. Typing follows the
same message-read boundary.

Room visibility and administrative membership facts update the process-wide
visibility cache. Its stable admission boundary includes room creation,
deletion, Universal changes, joins, leaves, member additions, member removals,
bans, and unbans. Facts for a room that a caller never saw are suppressed.

RBAC facts can revoke access without a room fact. The server closes affected
connections with `projection_reset_required`. The next subscription uses
current authorization. The frontend also scrubs known private resources
synchronously when a canonical event shows a direct authorization loss. It
then uses the authoritative resource read to converge.

A durable mapping or snapshot failure closes the connection before the cursor
advances. Reconnect retries the fact or uses a safe fallback. Unknown canonical
event variants are additive and can be ignored while the transport cursor
advances.

## Process-wide live ingress

`MyEventsHub` owns one NATS Core subscription to `live.sync.>` and one to
`live.evt.>` for each Chatto process. It classifies and decodes messages once,
waits for projections once, and fans immutable envelopes to bounded
per-session queues. Sessions for one user share room-visibility state. There
are no per-client NATS subscriptions or JetStream consumers.

Transient `live.sync.>` messages and durable `live.evt.>` messages use
`chatto.core.evt.v1.Event`. Transient variants use oneof tags 20000 through
29999. During a rolling upgrade, one transient wire message also contains the
matching previous `chatto.core.live.v1.LiveEvent` oneof tag. Old replicas read
that tag and ignore the canonical tag. New replicas read the canonical tag and
ignore the old tag. The hub also converts messages that contain only the old
envelope.

A NATS continuity gap or projection-readiness failure quarantines the hub and
closes current sessions. The replica admits a new hub generation only after
NATS resources, projections, and volatile watchers are current. A slow session
that exceeds its queue count or byte limit closes independently.

## Bundled frontend

The bundled frontend selects `SNAPSHOT`. It resets its server projection when
`subscribed.recovery_mode` is `SNAPSHOT`, applies resource chunks, and then
applies canonical events. It saves an event cursor only after its reducer
accepts the complete event.

The projection stores canonical public resources. It does not store
realtime-specific resource copies. Resource invalidation events start
coalesced ConnectRPC reads. If another event reaches the same resource family
during a read, the frontend runs one follow-up read. Timeline-derived stores
refresh their current ConnectRPC windows. A failed refresh does not make the
transport cursor unsafe because a later snapshot restores current resources.

The browser keeps one in-memory resource snapshot and cursor for each
authenticated server. Only the active server keeps a persistent socket.
Inactive servers use bounded periodic catch-up sockets. A page reload starts
without a cursor and receives a new snapshot.

The frontend keeps its snapshot during access-token rotation, cookie-session
renewal, server switches, network reconnects, and tab wake. It replaces the
socket and sends the same cursor. Human bearer credentials close at
access-token expiry. Cookie connections close at the renewal boundary. The
server revalidates the accepted credential before subscription and once per
minute.

The browser resets its liveness timer on every server frame. It replaces a
socket after a heartbeat stall. An undecodable or unknown top-level frame
causes a reconnect without cursor advancement. WebSocket connections use small
buffers and a shared write-buffer pool. When compression is enabled, the
server uses Huffman-only DEFLATE for frames of at least 1 KiB.

## Interface boundary

| Endpoint | Frame schema | Authorization | Description |
| --- | --- | --- | --- |
| `/api/realtime` | `chatto.realtime.v1.Realtime*` binary protobuf frames | Bearer credential in `hello` or a same-origin cookie; current resource and room visibility apply before mapping | Protocol 4 authorized canonical events, canonical snapshot resources, and bounded resume |

Realtime does not replace `chatto.api.v1`. ConnectRPC remains the public API
for commands, explicit resource reads, pagination, history, search, and
read-your-writes responses.
