# FDR-045: Realtime Event Stream

**Status:** Experimental
**Last reviewed:** 2026-08-31

## Overview

The realtime event stream gives authenticated clients an authorized live view
of one Chatto server. Bots, integrations, alternate clients, and the bundled
frontend use the same semantic event contract. The bundled frontend also uses
the stream to build and maintain its local server projection.

## Behavior

- A client opens one authenticated realtime subscription for a server.
- A subscription selects `SNAPSHOT` or `LIVE_ONLY` initial state. Snapshot
  clients receive authorized current state when resume is not possible.
  Live-only clients start at a stated current boundary without historical
  events.
- The stream sends authorized copies of canonical events for durable activity,
  including messages, edits, retractions, reactions, membership, rooms,
  profiles, calls, and other public domain changes.
- Public events contain the canonical event ID, source time, visible actor ID,
  event payload, and an opaque resume cursor in the transport wrapper.
- The server omits internal events, removes storage-only fields, and can add
  authorized client-only `_plaintext` fields. It never sends raw EVT bytes.
- A client can ignore an event type that it does not use and still retain the
  event cursor.
- Typing, presence transitions, and other transient activity are live-only.
  A snapshot supplies current latest-value state when clients need it.
- A recently disconnected client can reconnect with its last safe cursor. The
  server sends later authorized durable events before it continues live.
- A missing, invalid, expired, unsafe, or expensive cursor uses the requested
  safe fallback. It does not cause partial or unlimited historical playback.
- Resume and snapshot delivery use the caller's current authorization. Deleted,
  retracted, or erased data does not return in its old form.
- A client must tolerate duplicate events and use the stable event ID for
  deduplication.
- The stream does not guarantee every intermediate transition after a client
  is offline beyond the bounded resume window.
- ConnectRPC remains the normal API for commands, explicit resource reads,
  pagination, history, and read-your-writes responses.

## Design Decisions

### 1. One canonical event vocabulary serves all clients

**Decision:** Durable EVT facts, transient NATS Core signals, bots, and the
bundled frontend use `chatto.core.evt.v1.Event`. The realtime transport carries
a new authorized and censored copy of that event. Chatto does not provide a
frontend-only mutation feed or a parallel public event payload.
**Why:** A message edit, reaction, or membership change has one public meaning.
One contract makes the API easier to learn and prevents client-specific event
models from disagreeing. Existing EVT compatibility also gives the public
event vocabulary a strong additive contract. See ADR-088.
**Tradeoff:** Every public event and field needs an explicit authorization and
surface decision. The storage publisher must reject delivery-only fields.

### 2. Initial state is explicit

**Decision:** A subscription selects `SNAPSHOT` or `LIVE_ONLY`. The bundled
frontend selects `SNAPSHOT` and applies later semantic events to that local
projection. An event-only bot selects `LIVE_ONLY` and uses ConnectRPC when it
needs current resources.
**Why:** A full ConnectRPC bootstrap would require many reads and would make it
harder to define one ordered state boundary. The snapshot keeps startup and
reconnect efficient for projection clients without forcing a large bootstrap
on every integration.
**Tradeoff:** Clients must choose whether current-state bootstrap is part of
their subscription.

### 3. Resume repairs recent connection gaps

**Decision:** Resume cursors provide bounded session recovery. An unusable or
expensive cursor selects the subscription's requested safe fallback.
**Why:** Short replay prevents visible state jumps during ordinary network and
credential reconnects. Strict bounds protect the server and keep realtime
separate from event-log export.
**Tradeoff:** A long-offline snapshot client can recover current state but can
miss intermediate transitions. A live-only client must fetch any required
current state through ConnectRPC.

### 4. Snapshots use resource chunks

**Decision:** Snapshot frames contain authorized current resources. The
subscription acknowledgement tells the client to replace local state, resource
chunks carry the snapshot, and `caught_up` marks its complete boundary.
Snapshots do not contain synthetic domain events.
**Why:** Resource chunks let the frontend bootstrap incrementally without
turning current state into fake history or exposing frontend cache operations.
**Tradeoff:** Snapshot clients need a separate reducer for current-state chunks.

### 5. Durable and transient activity have different recovery

**Decision:** Public durable events can resume from EVT. Transient activity is
live-only, and current latest-value values are reconciled through snapshots.
**Why:** Typing and presence transitions have no useful historical meaning.
Durable domain changes need ordering and short-gap recovery.
**Tradeoff:** A reconnect does not recreate transient presentation effects.

### 6. ConnectRPC remains the explicit resource API

**Decision:** Realtime events carry enough authorized identity and context to
describe a change. Clients use ConnectRPC when they need an explicit resource,
large collection, history page, command response, or additional detail.
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
  (process-wide realtime event hub), ADR-087 (semantic realtime events),
  ADR-088 (one event vocabulary)
- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-005 (Reactions), FDR-010
  (Typing Indicators), FDR-011 (User Presence), FDR-012 (Notifications),
  FDR-016 (Voice Calls), FDR-019 (Room Lifecycle), FDR-022 (User Profile),
  FDR-031 (Client–Server Compatibility Discovery), FDR-038 (Bot Accounts)

## Open Questions

- Define whether reliable long-offline automation first uses outgoing
  webhooks or a paged public activity API.
- Set the resume age, scan, event, byte, and time budgets from measurements.
