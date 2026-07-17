# Realtime Delivery Inventory

Key files: [`proto/chatto/realtime/v1/realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto), [`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go), [`cli/internal/http_server/realtime_projection.go`](../../cli/internal/http_server/realtime_projection.go), [`cli/internal/connectapi/realtime_projection.go`](../../cli/internal/connectapi/realtime_projection.go), [`cli/internal/core/realtime_replay.go`](../../cli/internal/core/realtime_replay.go), [`apps/frontend/src/lib/state/server/projection.svelte.ts`](../../apps/frontend/src/lib/state/server/projection.svelte.ts)

Related decisions: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md) and [ADR-051](../adr/ADR-051-server-scoped-resumable-client-projection.md).

The protobuf realtime API is mounted at `GET /api/realtime` and upgrades to a
binary WebSocket. The first client frame must be `hello`; the server accepts
only protocol version 2 and authenticates either the hello bearer token or an
existing cookie session. The second client frame must be `subscribe_events`.

The `chatto.realtime.v1` package name is the protobuf namespace, not the
behavioural protocol version. Protocol 2 is the server-scoped projection stream. It uses
`RealtimeProjectionEvent`, an optional resume cursor on `subscribe_events`, and
`caught_up` at the replay-to-live boundary. Application heartbeats and client
`ping`/server `pong` share the same connection.

The bundled client creates its event-bus reducer before discovery completes so
consumers can register synchronously, but it opens the WebSocket only after
discovery advertises `chatto.realtime.projection.v1`. Servers older than 0.5 do
not advertise that required contract and are reported as unsupported rather
than receiving the former ConnectRPC bootstrap plus protocol-v1 live feed. An
`unsupported_protocol` error is terminal for the current bus and does not enter
the reconnect loop.

## Compacted projection prefix

A subscription without a usable cursor emits one ordered stream of
idempotent operations:

- `reset`;
- current public server profile, authenticated server presentation/runtime
  state, and authenticated viewer state;
- every public server directory user;
- every room visible to the viewer, its complete membership as references into
  the server user directory, and the complete visible room-group layout;
- the latest 50 renderable timeline events for every joined room;
- the newest finite pending-notification page and complete per-room counts.
- every active call visible to the viewer.

The snapshot builder uses the same ConnectRPC assemblers as public reads. It
decrypts PII only at the authenticated response boundary and resolves messages
through current deletion and key-shredding projections. Deleted or
crypto-erased bodies therefore appear only as normal tombstones. Timeline
windows are assembled concurrently with bounded concurrency.

The frontend applies this prefix and every later event through the same
`ServerProjectionStore` reducer. Server profile, MOTD, and runtime capability
changes replace canonical projection state instead of causing a ConnectRPC
refresh. Canonical timeline pages evict rows beyond their newest 50, while
heavier message stores are created lazily only for rooms the UI selects.
Timeline replacements carry an opaque cursor for every retained row, and later
row upserts carry that row's cursor. The reducer can therefore advance its
pagination boundary using only the projection stream.
Changing the route selects retained state for rendering and does not initiate
initial room hydration. Room-member lists and DM labels resolve those membership
references through the already-warm user projection instead of issuing a
second bootstrap query.

## Resume and live handoff

The sealed cursor contains an EVT stream incarnation, global sequence, and
viewer binding. XChaCha20-Poly1305 protects it with a purpose-separated key
derived from `core.secret_key`; random nonces prevent equal payloads producing
equal tokens. NATS and JetStream coordinates are never public API facts.
Tampering, cross-user reuse, secret rotation, or foreign stream incarnation
selects a compacted reset. The browser retains a cursor only with its
corresponding in-memory projection. Socket
reconnects can resume; page reloads and recreated stores omit it and receive a
new compacted prefix.

For a valid short gap, the handler subscribes to the process-wide live hub,
captures an EVT cutoff, and performs bounded JetStream point reads for the
sequences after the cursor. It does not create a JetStream consumer. Each
deliverable room, asset, or user fact waits for its owning projection and is
converted to current public resource operations. The handler sends `caught_up`
at the cutoff, discards buffered live duplicates through that sequence, and
continues with the hub stream.

Because pending notifications live in `RUNTIME_STATE`, every valid resume also
emits a current `notifications_replace` operation before `caught_up`. Buffered
notification create/dismiss signals cover mutations concurrent with that
finite reconciliation.

Replay scans at most 10,000 EVT sequences and emits at most 2,000 durable
facts. Missing, malformed, expired, foreign-incarnation, oversized, or
authorization-sensitive gaps select the compacted prefix instead of failing
the subscription.

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

Message facts carry a lightweight replacement of the affected room's viewer
state alongside timeline mutations. Notification counts converge through
notification signals and the finite resume replacement. Message delivery does
not scan notification state or reassemble and retransmit room metadata and
complete membership. Echo tombstone upserts explicitly distinguish
canonical-reply deletion from direct echo removal.

Room-read signals emit a `RoomViewerStateReplace` for the affected room and a
finite `NotificationsReplace`. This keeps the retained canonical room row,
pending-notification state, and both sidebar indicators in step, so a later
mutation cannot restore stale unread or mention state. Root-message timeline
upserts also advance the affected room in the retained activity order; later
viewer-state replacements therefore cannot undo DM sorting.

A durable projection hydration or mapping failure closes the session
without advancing its cursor. Reconnect retries that EVT sequence or selects a
compacted reset, so a later cursor cannot make a dropped mutation permanent.
Historical message creation for an echo that is hidden in current projection
state maps to an idempotent timeline removal. Asset processing and deletion
facts map to authoritative upserts of their owning message and any visible
channel echo, so replay never advances beyond a durable attachment mutation
without applying its current render state.

Transient/latest-value signals such as presence, typing, and notification
create/dismiss hints continue as `RealtimeEventEnvelope` frames on the same
WebSocket. Active calls instead converge through `active_calls_replace` in the
compacted prefix and after every durable call transition. Transient frames have no durable
cursor; finite pending-notification state is reconciled explicitly on resume,
while other transient values are not part of canonical projection replay. The
process-wide PresenceHub retains current presence and fans out later
transitions.

Process-wide ingress loss or projection-readiness failure quarantines the hub
and closes every session. A slow session that exceeds its queue limits is
closed independently. Both cases reconnect through resume or a compacted reset
rather than continuing a healthy-looking stream across an unobservable gap.

WebSocket connections use small read/write buffers and share a write-buffer
pool. When compression is enabled, the server uses Huffman-only DEFLATE and
compresses frames of at least 1 KiB.

| Endpoint | Frame schema | Authorization | Description |
| --- | --- | --- | --- |
| `/api/realtime` | `chatto.realtime.v1.Realtime*` binary protobuf frames | Bearer token in hello or cookie auth; current per-resource and room visibility is applied before public projection mapping. | Protocol 2 server-scoped compacted/resumable projection delivery. |
