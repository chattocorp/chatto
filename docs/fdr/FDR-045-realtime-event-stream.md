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
- A subscription selects `RESOURCE_READS` or `LIVE_ONLY` initial state.
  Resource clients read the current state that they need through ConnectRPC
  when resume is not possible. Live-only clients start at a stated current
  boundary without historical events.
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
  ConnectRPC supplies current latest-value state when clients need it.
- A recently disconnected client can reconnect with its last safe cursor. The
  server sends authorized durable events after that cursor before it continues
  live.
- A missing, invalid, expired, unsafe, or expensive cursor uses the requested
  safe fallback. It does not cause partial or unlimited historical playback.
- Resume and cursor-bounded resource reads use the caller's current
  authorization. Deleted, retracted, or erased data does not return in its old
  form.
- A client must tolerate duplicate events and use the stable event ID for
  deduplication.
- A client that starts a new resource reset must discard responses from the
  earlier reset. It must not let a late response replace newer state.
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
Field surfaces are static exposure classes. Viewer-specific field policy is not
part of this version. Event authorization must make all delivered fields safe;
a future narrower field needs an explicit viewer-aware projection rule or a
separate authorized shape.

### 2. Initial state is explicit

**Decision:** A subscription selects `RESOURCE_READS` or `LIVE_ONLY`. The
bundled frontend selects `RESOURCE_READS` and reads only the canonical
resources that it keeps. An event-only bot selects `LIVE_ONLY` and uses
ConnectRPC only when it needs current resources.
**Why:** One explicit choice tells the server whether the client needs a safe
current-state boundary. It does not force a large bootstrap on integrations
that need only events.
**Tradeoff:** A resource client must make several ConnectRPC calls before it
asks the WebSocket to catch up.

### 3. Resume repairs recent connection gaps

**Decision:** Resume cursors provide bounded session recovery. An unusable or
expensive cursor selects the subscription's requested safe fallback.
**Why:** Short replay prevents visible state jumps during ordinary network and
credential reconnects. Strict bounds protect the server and keep realtime
separate from event-log export.
**Tradeoff:** A long-offline resource client can recover current state but can
miss intermediate transitions. A live-only client must fetch any required
current state through ConnectRPC.

### 4. ConnectRPC reads and events close one state interval

**Decision:** The server gives a resource client an opaque boundary `E`. The
client clears its retained state and reads the required canonical resources
through ConnectRPC with `E` as the minimum cursor. The client then requests
catch-up. The server sends events after `E` through `F`, then sends
`caught_up(F)`. Each serving replica waits until its local projections include
`E` before it answers a bounded read.
**Why:** This closes races between replicas without putting resource shapes in
the WebSocket protocol. Each resource has one public API and one compatibility
contract.
**Tradeoff:** The server must validate cursor-bound read headers and hold a
bounded subscriber queue while the client reads resources.

Clients must give cursor-bounded reads finite deadlines. A projection that
cannot reach the requested cursor must cause reconnect or safe fallback. It
must not stop the client at one boundary for an unlimited time.

The bundled frontend does not read the complete user directory. It reads the
viewer, rooms, room groups, notification page, active calls, and server state.
It uses bounded user batch reads only for users referenced by direct messages
or later events. It also reloads each mounted room or thread timeline at the
same boundary. Unmounted timelines remain lazy.

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
  (process-wide realtime event hub), ADR-087 (semantic realtime events),
  ADR-088 (one event vocabulary)
- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-005 (Reactions), FDR-010
  (Typing Indicators), FDR-011 (User Presence), FDR-012 (Notifications),
  FDR-016 (Voice Calls), FDR-019 (Room Lifecycle), FDR-022 (User Profile),
  FDR-031 (Client–Server Compatibility Discovery), FDR-038 (Bot Accounts)

## Open Questions

- Define whether reliable long-offline automation first uses outgoing
  webhooks or a paged public activity API.
- Validate the 24-hour cursor lifetime, 10,000-position scan cap, 2,000-event
  delivery cap, and 30-second catch-up limit against production measurements.
  Add a byte cap if event-size measurements show that event count is not a
  sufficient transport bound.
