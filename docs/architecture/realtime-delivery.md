# Realtime Delivery Inventory

Key files:

- [`proto/chatto/realtime/v1/realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto)
- [`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go)
- [`cli/internal/core/my_events_model.go`](../../cli/internal/core/my_events_model.go)
- [`apps/frontend/src/lib/state/server/eventBus.svelte.ts`](../../apps/frontend/src/lib/state/server/eventBus.svelte.ts)
- [`apps/frontend/src/lib/presenceTracking.ts`](../../apps/frontend/src/lib/presenceTracking.ts)

Related decision: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md).

The protobuf realtime API is mounted at `GET /api/realtime` and upgrades to a
binary WebSocket protocol. The first client frame must be
`RealtimeClientFrame.hello`. The server accepts protocol version 1,
authenticates either the hello bearer token or the existing cookie session, and
replies with `RealtimeServerFrame.hello`.

The second client frame must be `subscribe_events`. The server then sends
`subscribed` and starts forwarding authorized `RealtimeEventEnvelope` frames
plus application-level heartbeats. Clients can send `ping` frames and receive
`pong`. The v1 protocol is live-only and exposes no acknowledgement frames,
resume requests, or event cursors.

The HTTP handler does not implement independent room/RBAC filtering. After
authentication it calls `core.StreamMyEventsWithOptions` with legacy presence
touching disabled, then maps the already-authorized core envelope into public
`chatto.realtime.v1` frames. Room membership, DM privacy, server/user/config
event gates, projection readiness, live membership changes, slow-consumer
shutdown, and session termination therefore remain shared with
`core.StreamMyEvents`.

The frontend keeps an authenticated server's realtime stream connected
independently of the local presence mode. "Look offline" stops presence
refreshes and lets the live presence record expire; it does not pause event
delivery. Realtime connection establishment itself does not touch presence.

A process-wide `MyEventsHub` owns one NATS Core subscription to `live.sync.>`
and one to `live.evt.>`. It classifies subjects before decoding, waits for
projections once, and fans immutable decoded events into count- and byte-bounded
session queues. Sessions for one user share room-visibility state.

The process-wide PresenceHub retains the current presence snapshot and fans out
only future status transitions. Individual streams do not copy the server-wide
snapshot. Queue overflow closes only the affected stream, allowing reconnect
catch-up to restore current projected or latest-value state.

WebSocket connections use small read/write buffers and share a write-buffer
pool, so idle connections do not retain a full default transport buffer in each
direction. When compression is enabled, the server uses Huffman-only DEFLATE
and compresses only frames of at least 1 KiB. The small invalidation and
heartbeat frames that dominate realtime traffic remain uncompressed and do not
instantiate Gorilla's server-side compressor state.

Realtime events are public API signals, not raw persisted `corev1.Event` or
`corev1.LiveEvent` payloads. ID-only payloads are invalidation signals. The
realtime protobuf documents the intended `chatto.api.v1` or `chatto.admin.v1`
ConnectRPC hydration path for each referenced resource, so clients can recover
missed live state through projected reads.

Clients recover missed state with projected reads, the same projected-read catch-up model. There is no per-connection JetStream consumer and no public subscription replay cursor.

| Endpoint        | Frame schema                          | Auth / authorization                                                                 | Description |
| --------------- | ------------------------------------- | ------------------------------------------------------------------------------------ | ----------- |
| `/api/realtime` | `chatto.realtime.v1.Realtime*` binary protobuf frames | Bearer token in hello or cookie auth; delivery delegated to `StreamMyEvents` for per-event authorization. | Live-only authenticated event stream for messages, reactions, typing, presence, rooms, notifications, read state, server/user profile invalidation, and session termination. |
