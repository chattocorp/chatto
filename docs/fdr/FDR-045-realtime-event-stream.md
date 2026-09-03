# FDR-045: Realtime Event Stream

**Status:** Experimental
**Last reviewed:** 2026-09-01

## Overview

The realtime event stream gives authenticated clients an authorized live view
of one Chatto server. Bots, integrations, alternate clients, and the bundled
frontend use the same semantic event contract. The bundled frontend also uses
the stream to build and maintain its local server projection.

## Behavior

- A client opens one authenticated realtime subscription for a server.
- A subscription selects `SNAPSHOT` or `LIVE_ONLY` initial state. Snapshot
  clients receive current canonical resources through the WebSocket when
  resume is not possible. Live-only clients start at a stated current boundary
  without historical events.
- The stream sends authorized public events for durable activity,
  including messages, edits, retractions, reactions, membership, rooms,
  profiles, calls, and other public domain changes.
- Public events contain the canonical event ID, source time, visible actor ID,
  a canonical payload message, and an opaque resume cursor in the transport
  wrapper.
- The server omits internal events, removes storage-only fields, and can add
  authorized client-only `_plaintext` fields. It never sends raw EVT bytes.
- A client can ignore an event type that it does not use and still retain the
  event cursor.
- Typing, presence transitions, and other transient activity are live-only.
  ConnectRPC supplies current latest-value state when clients need it.
- A recently disconnected client can reconnect with its last safe cursor. The
  server sends authorized durable events after that cursor before it continues
  live.
- A missing, invalid, expired, unsafe, or expensive cursor uses the requested
  safe fallback. It does not cause partial or unlimited historical playback.
- A live durable content fact that has no safe public event projection closes
  the current projection stream. Snapshot clients reconnect and receive exact
  current state instead of relying on a cross-replica transient read.
- Resume, snapshots, and cursor-bounded resource reads use the caller's current
  authorization. Deleted, retracted, or erased data does not return in its old
  form.
- A client must tolerate duplicate events and use the stable event ID for
  deduplication.
- A client must discard an incomplete snapshot. A new snapshot also starts a
  new local projection generation. Late reads from an earlier generation must
  not replace newer state.
- The stream does not guarantee every intermediate transition after a client
  is offline beyond the bounded resume window.
- ConnectRPC remains the normal API for commands, explicit resource reads,
  pagination, history, and read-your-writes responses.

## Design Decisions

### 1. One semantic event vocabulary serves all clients

**Decision:** Durable EVT facts and transient NATS Core signals use
`chatto.core.evt.v1.Event`. Realtime uses the explicit
`chatto.realtime.v1.PublicEvent` union. Each public member reuses the matching
canonical payload message, name, and field number. Chatto does not provide a
frontend-only mutation feed or duplicate public payload messages.
**Why:** A message edit, reaction, or membership change has one public meaning.
One contract makes the API easier to learn and prevents client-specific event
models from disagreeing. Existing EVT compatibility also gives reused payload
messages a strong additive contract. The public union makes event exposure
visible in the schema. See ADR-091 and ADR-092.
**Tradeoff:** Every public event and field needs an explicit authorization and
surface decision. The storage publisher must reject delivery-only fields.
Field surfaces are static exposure classes. Viewer-specific field policy is not
part of this version. Event authorization must make all delivered fields safe;
a future narrower field needs an explicit viewer-aware projection rule or a
separate authorized shape.

### 2. Initial state is explicit

**Decision:** A subscription selects `SNAPSHOT` or `LIVE_ONLY`. The bundled
frontend selects `SNAPSHOT`. The server sends an exact authorized snapshot of
`ServerContentView` before events after the same boundary. An event-only bot
selects `LIVE_ONLY` and uses ConnectRPC only when it needs a resource.
**Why:** One WebSocket replica can capture the content snapshot and event
boundary together. Clients do not have to coordinate a bootstrap across
replicas. Integrations that need only events do not receive the snapshot.
**Tradeoff:** The WebSocket protocol contains a small snapshot wrapper. The
snapshot must stay bounded and must not become a second resource hierarchy.

### 3. Resume repairs recent connection gaps

**Decision:** Resume cursors provide bounded session recovery. An unusable or
expensive cursor selects the subscription's requested safe fallback.
**Why:** Short replay prevents visible state jumps during ordinary network and
credential reconnects. Strict bounds protect the server and keep realtime
separate from event-log export.
**Tradeoff:** A long-offline snapshot client can recover current state but can
miss intermediate transitions. A live-only client must fetch any required
current state through ConnectRPC.

### 4. One exact snapshot and one event boundary close the startup interval

**Decision:** The WebSocket replica registers for live delivery and captures
an exact snapshot at EVT boundary `E`. It sends `subscribed(SNAPSHOT)`, one
snapshot frame for each bounded resource family, and then `caught_up(E)`.
Buffered durable events after `E` follow in order. The snapshot reuses
canonical `chatto.api.v1` resource messages.

The snapshot contains the public server profile, complete visible room
directory, complete visible room-group layout, complete visible active-call
state, the viewer, and users referenced by snapshot resources. It does not
contain the complete user directory, timelines, search results, files, pins,
notifications, presence, read markers, or account-security state.

**Why:** The same replica owns the snapshot and event handoff. This removes the
cross-replica bootstrap race and the client-driven second catch-up interval.
Reused resource messages prevent a second state schema.
**Tradeoff:** Snapshot framing is part of the WebSocket protocol. State that is
not in the bounded content view still needs explicit ConnectRPC reads.

After an event, a targeted ConnectRPC read can use that event cursor as its
minimum boundary. Each serving replica validates the cursor and waits until
its content view includes that boundary. Clients must give these reads finite
deadlines and must not retain the event cursor until required reconciliation
succeeds.

The bundled frontend reads notifications and other auxiliary state after the
snapshot. It reloads only mounted room or thread timelines. Unmounted
timelines remain lazy. A new snapshot invalidates any late response from an
earlier projection generation.

### 5. Durable and transient activity have different recovery

**Decision:** Public durable events can resume from EVT. Transient activity is
live-only, and current latest-value values are reconciled through resource
reads.
**Why:** Typing and presence transitions have no useful historical meaning.
Durable domain changes need ordering and short-gap recovery.
**Tradeoff:** A reconnect does not recreate transient presentation effects.

### 6. ConnectRPC remains the explicit resource API

**Decision:** Realtime events carry enough authorized identity and context to
describe a change. Clients use ConnectRPC when they need an explicit resource,
large collection, history page, command response, or additional detail.
An authorized message-post event can include its client-only plaintext body so
the client can show the post before it reads the complete message resource.
The bundled frontend first creates a temporary row with the event metadata,
reply references, actor, and plaintext. It leaves resource-only values empty,
then replaces the row after `GetMessage` returns attachments, previews,
reactions, pin state, thread details, and the timeline cursor.
**Why:** Events should not become unbounded resource dumps. Complete public
resource APIs also let simple bots avoid maintaining the complete frontend
projection.
**Tradeoff:** Some integrations make a follow-up read after an event. Public
APIs need bounded batch reads when events commonly reference many resources.

### 7. Reliable long-offline automation is a separate feature

**Decision:** The realtime stream does not claim durable delivery beyond its
resume window. A future acknowledged webhook or paged activity feature can
provide that guarantee and reuse the public event vocabulary.
**Why:** Durable automation needs acknowledgement, retry, retention, and
operator-visible failure semantics. A browser-style WebSocket does not provide
those semantics safely.
**Tradeoff:** The first semantic realtime version does not satisfy integrations
that must process every transition after a long outage.

## Related

- **ADRs:** ADR-012 (two-tier realtime events), ADR-026 (event identity),
  ADR-033 (event-sourced state), ADR-034 (single event stream), ADR-042
  (protobuf-first public API), ADR-045 (public API stability), ADR-049
  (process-wide realtime event hub), ADR-090 (semantic realtime events),
  ADR-091 (one event vocabulary), ADR-092 (public realtime event union)
- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-005 (Reactions), FDR-010
  (Typing Indicators), FDR-011 (User Presence), FDR-012 (Notifications),
  FDR-016 (Voice Calls), FDR-019 (Room Lifecycle), FDR-022 (User Profile),
  FDR-031 (Client–Server Compatibility Discovery), FDR-038 (Bot Accounts)

## Open Questions

- Define whether reliable long-offline automation first uses outgoing
  webhooks or a paged public activity API.
- Validate the 15-minute cursor lifetime, 10,000-position scan cap, 2,000-event
  delivery cap, and 30-second catch-up limit against production measurements.
  Add a byte cap if event-size measurements show that event count is not a
  sufficient transport bound.
