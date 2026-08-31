# Realtime Delivery Inventory

Key files:

- [`proto/chatto/realtime/v1/realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto)
- [`proto/chatto/core/evt/v1/event.proto`](../../proto/chatto/core/evt/v1/event.proto)
- [`proto/chatto/core/event/v1/options.proto`](../../proto/chatto/core/event/v1/options.proto)
- [`proto/chatto/core/live/v1/live_events.proto`](../../proto/chatto/core/live/v1/live_events.proto)
- [`cli/internal/core/my_events_hub.go`](../../cli/internal/core/my_events_hub.go)
- [`cli/internal/core/realtime_replay.go`](../../cli/internal/core/realtime_replay.go)
- [`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go)
- [`cli/internal/http_server/realtime_projection.go`](../../cli/internal/http_server/realtime_projection.go)
- [`apps/frontend/src/lib/eventBus.svelte.ts`](../../apps/frontend/src/lib/eventBus.svelte.ts)
- [`apps/frontend/src/lib/state/server/projection.svelte.ts`](../../apps/frontend/src/lib/state/server/projection.svelte.ts)

Related decisions: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md),
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md),
[ADR-084](../adr/ADR-084-separate-internal-protobufs-by-storage-contract.md),
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

The second client frame is `subscribe_events`. It contains:

- an optional opaque resume cursor;
- zero to 64 retained room IDs; and
- a required initial-state choice of `SNAPSHOT` or `LIVE_ONLY`.

The server replies with `subscribed`. Its recovery mode is one of:

- `SNAPSHOT`: the server sends authorized current-state items;
- `LIVE_ONLY`: the server starts at the current boundary without state or
  history; or
- `RESUME`: the server sends authorized durable events after the cursor.

The server then sends `caught_up` with the live handoff cursor. The client can
consider the subscription current only after this frame.

## Canonical events

`chatto.core.evt.v1.Event` is the semantic unit for durable facts and transient
signals. `RealtimeEvent` is its public transport wrapper. Bots, integrations,
alternate clients, and the bundled frontend receive the same event vocabulary.

A durable event has a stable event ID, source time, visible actor ID, and one
event variant. The transport wrapper adds an opaque resume cursor. Variants cover
messages, reactions, pins, assets, rooms, room membership, threads, users,
calls, and public invalidation events. Typing, presence changes, and session
termination use the same envelope but have no resume cursor.

Common metadata is outside the event `oneof`, and the cursor is outside the
canonical Event. A client can
ignore a new event variant and still retain its cursor after it accepts the
complete frame. The server creates a fresh authorized event for delivery. A
public catalogue omits internal variants. Protobuf field-surface options allow
shared, storage-only, and client-only fields. Unspecified payload fields are
denied by default. The copier does not retain unknown fields. Authorized
delivery-only decrypted values use `_plaintext` fields. Public events do not
expose raw EVT bytes, ciphertext, subjects, stream identities, or sequence
numbers.

An event can include authorized `RealtimeStateItem` values. These values show
current resources after the event. They help a projection client converge, but
they do not change the semantic event. An event-only bot can ignore them.

Retained room IDs affect only timeline state hydration. They never filter
semantic events. For example, a bot receives an authorized message or reaction
event for an unretained room, but the event does not include that room's current
timeline row.

## Snapshot and current state

A `SNAPSHOT` recovery sends one `state` frame for each authorized current-state
item. Snapshot rows are not synthetic events and do not have event cursors. The
`subscribed.recovery_mode` value tells the client to replace its projected
state before it applies the rows.

The snapshot can include:

- the public server profile and authenticated runtime state;
- the viewer and visible user directory;
- lightweight visible rooms and the room-group layout;
- notification state and active calls; and
- recent timelines for retained rooms.

The snapshot keeps channel membership and timelines lazy. It includes DM
participant references because a DM needs them for its label and access model.
The server uses the same ConnectRPC assemblers as explicit public reads. It
uses current deletion and key-shredding state, so erased plaintext does not
return during recovery.

Every snapshot or resume also sends a finite latest-value reconciliation before
`caught_up`. It refreshes viewer data, room and thread viewer state,
notifications, and presence. A snapshot uses the room-marker change fence to
send only room markers that changed while the snapshot was built. A resume
refreshes all visible room viewer state because EVT cannot reconstruct runtime
read markers.

## Lazy room hydration

After `caught_up`, a client can send `hydrate_room` for a joined room. The
server replies with standalone `room` and `room_timeline` state frames. The
room state contains the complete current member ID set. Later events include
current timeline-row state only while that room is retained.

The client sends confirmed retained room IDs in its next subscription. Both
client and server limit the set to 64 room IDs. A repeated hydration request is
idempotent. Non-fatal hydration errors identify the room and can include a
retry delay.

Hydration uses the process-wide catch-up semaphore. The server serializes it
per user. The process-local token bucket permits a burst of 20 requests and
restores one token each second.

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
5. Apply current authorization and censor each public canonical event.
6. Send finite latest-value reconciliation state.
7. Send `caught_up`, discard buffered duplicates through the boundary, and
   continue with live delivery.

The direct-read path creates no JetStream consumer. It scans at most 10,000 EVT
sequences and emits at most 2,000 durable events. The complete catch-up has a
30-second deadline.

A missing, invalid, expired, foreign-stream, oversized, or authorization-unsafe
cursor selects the requested fallback. A `SNAPSHOT` client receives current
state. A `LIVE_ONLY` client starts at the current boundary and receives no old
events. The server never sends a partial replay and then silently skips to live
delivery.

Incremental replay and fallback share one process-local admission guard. Each
replica admits at most eight catch-ups at once and one at a time for each user.
Stale-cursor replay has a per-user burst of three and restores one token every
20 seconds. Cursorless and current-boundary catch-ups use the general burst of
20 and restore one token each second. Metrics expose active, started, timed-out,
and rejected catch-ups.

## Authorization and projection readiness

Live delivery, resume, state hydration, and snapshots use current
authorization. Message and asset events require room membership. A channel
viewer also needs `message.read`, or `message.read-interactions` with a
relationship to the canonical thread root. DM membership authorizes the read.
Typing follows the same message-read boundary.

Room visibility and administrative membership facts update the process-wide
visibility cache. Its stable admission boundary includes room creation,
deletion, Universal changes, joins, leaves, member adds, member removals, bans,
and unbans. Facts for a room that a caller never saw are suppressed. A caller
that loses visibility receives current removal state or must reconnect for a
safe snapshot.

RBAC facts can revoke access without a room fact. The server closes affected
connections with `projection_reset_required`; the next subscription uses
current authorization. A self-authored change that cannot change the writer's
authorization can advance that writer's cursor without a rebuild.

A durable mapping or state-hydration failure closes the connection before the
cursor advances. Reconnect retries the fact or uses a safe fallback. Unknown
public event variants are different: they are additive and can be ignored while
the common cursor advances.

## Process-wide live ingress

`MyEventsHub` owns one NATS Core subscription to `live.sync.>` and one to
`live.evt.>` for each Chatto process. It classifies and decodes messages once,
waits for projections once, and fans immutable envelopes to bounded per-session
queues. Sessions for one user share room-visibility state. There are no
per-client NATS subscriptions or JetStream consumers.

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
`subscribed.recovery_mode` is `SNAPSHOT`, applies standalone state frames, and
then applies state attached to canonical events. It saves an event cursor only
after its projection reducer accepts the complete event.

Root message events update room recency and first-message visibility. Reaction
and pin events drive their specific UI
effects. Unknown additive event and state variants do not stop known state from
applying or prevent the common cursor from advancing.

The browser keeps one in-memory projection and cursor for each authenticated
server. Only the active server keeps a persistent socket. Inactive servers use
bounded periodic catch-up sockets. A page reload starts without a cursor and
receives a new snapshot.

The frontend keeps its projection during access-token rotation, cookie-session
renewal, server switches, network reconnects, and tab wake. It replaces the
socket and sends the same cursor and retained-room set. It does not start a
parallel ConnectRPC bootstrap for canonical projection state.

Human bearer credentials close at access-token expiry. Cookie connections
close at the renewal boundary. OAuth-client blocks and individual bot API-key
revocations close only matching sessions. The server revalidates the accepted
credential before subscription and once per minute.

The browser resets its liveness timer on every server frame. It replaces a
socket after a heartbeat stall. An undecodable or unknown top-level frame
causes a reconnect without cursor advancement. WebSocket connections use small
buffers and a shared write-buffer pool. When compression is enabled, the
server uses Huffman-only DEFLATE for frames of at least 1 KiB.

## Interface boundary

| Endpoint | Frame schema | Authorization | Description |
| --- | --- | --- | --- |
| `/api/realtime` | `chatto.realtime.v1.Realtime*` binary protobuf frames | Bearer credential in `hello` or a same-origin cookie; current resource and room visibility apply before mapping | Protocol 4 authorized canonical events, explicit snapshot or live-only startup, lazy room state, and bounded resume |

Realtime does not replace `chatto.api.v1`. ConnectRPC remains the public API
for commands, explicit resource reads, pagination, history, search, and
read-your-writes responses.
