# Realtime Delivery Inventory

Key files:

- [`realtime.proto`](../../proto/chatto/realtime/v1/realtime.proto)
- [`events.proto`](../../proto/chatto/realtime/v1/events.proto)
- [`realtime.go`](../../cli/internal/http_server/realtime.go)
- [`realtime_consistency.go`](../../cli/internal/connectapi/realtime_consistency.go)
- [`eventBus.svelte.ts`](../../apps/frontend/src/lib/state/server/eventBus.svelte.ts)
- [`realtimeResources.ts`](../../apps/frontend/src/lib/api-client/realtimeResources.ts)

Related decisions: [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md),
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md),
[ADR-091](../adr/ADR-091-semantic-realtime-events-with-bounded-resume.md),
[ADR-093](../adr/ADR-093-use-a-public-realtime-event-union.md),
[ADR-094](../adr/ADR-094-separate-durable-and-pubsub-event-envelopes.md), and
[ADR-095](../adr/ADR-095-direct-message-permission-scope-and-threads.md).

## Public protocol

The integration client [`@chatto/client`](../../packages/chatto-client/README.md)
also consumes this protocol. It uses bearer authentication, live-only fallback,
ordered async event acceptance, bounded buffering, and process-local resume
cursors. Terminal errors stop consumption; unavailable replay reports a gap.
The [bot client](../../packages/chatto-bot-client/README.md) supplies addressing
recognition and process-local accepted-delivery tracking. The
[ChattoBot package](../../packages/chattobot/README.md) routes message events
into new or active Runling conversations. It accepts a delivery after inbox
insertion or successful run registration. Runling itself has no Chatto runtime
dependency.

The public API is a binary protobuf WebSocket at `GET /api/realtime`. The
server accepts behavioral protocol version 4. The `chatto.realtime.v1` suffix
is the protobuf package name. It is not the behavioral protocol version.

The client sends `RealtimeSubscribe` as its first binary WebSocket message. It
contains protocol version 4, an optional bearer credential, an optional opaque
resume cursor, and a required `SNAPSHOT` or `LIVE_ONLY` fallback choice. A
same-origin browser can use its cookie session. The client sends no more
application messages on the socket.

The server selects one recovery path:

- `SNAPSHOT`: send an exact authorized content snapshot;
- `LIVE_ONLY`: start at the current boundary without current state or old
  events; or
- `RESUME`: send authorized durable events after the supplied cursor.

The received frames show the selected path. The server then sends `caught_up`
with the handoff cursor. The client can consider the subscription current only
after it applies all earlier frames and this marker. The other server frames
are `event`, `heartbeat`, and `close`. All terminal protocol results use
`close`. WebSocket control frames provide ping and pong behavior.

## Public events

`chatto.core.evt.v1.Event` contains durable EVT facts.
`chatto.core.pubsub.v1.PubSubEvent` contains a restricted set of NATS Core
pubsub events. Client-facing variants reference the public payload messages
directly. Private controls, such as session termination, keep private payloads.
`chatto.realtime.v1.RealtimeEvent` is the authorized public event shape for
both sources. It contains common metadata, one public payload variant, and an
optional opaque resume cursor. It does not contain resource state.

A public event has a stable event ID, source time, visible actor ID, and one
event variant. Variants cover messages, reactions, pins, assets, rooms,
membership, threads, users, calls, and public invalidations. Typing and
presence changes use the same public union but have no resume cursor. Session
termination uses a `close` frame instead of an event.

`ViewerPresencePreferenceChanged` is delivered only to the account itself and
requests a private preference read. This transient signal has no cursor and is
not stored in EVT. Other viewers receive no frame for it. Public presence
transitions come from the effective-status hub. Invisible
heartbeats and expiry do not produce repeated Offline transitions. Typing is
checked against the private choice at publication and delivery.

Common metadata and the cursor are outside the event `oneof`. A client can
ignore a new event variant and still retain its cursor after it accepts the
complete frame.

The `RealtimeEvent.event` union and `events.proto` are the public catalogue.
Public names and compact field numbers do not expose whether the internal
source is EVT or pubsub. Public payload field numbers are independent from EVT
and from both envelope unions. A missing union member keeps an internal variant
out of the public API. A missing public payload field keeps an internal field
out of the generated client types.

After event authorization, an exhaustive typed mapper copies approved durable
values into a new dedicated public payload. For pubsub, the restricted private
union already contains the public payload type. The mapper selects the public
union arm and deep-copies the complete event before caller-specific filtering.
It adds trusted decrypted values to public-only `_plaintext` fields for durable
events. Public events do not
expose raw EVT bytes, ciphertext, nonces, storage pointers, private moderation
data, subjects, stream identities, or sequence numbers.

Event authorization must make every delivered field safe for that viewer. A
future field with narrower visibility needs an explicit viewer-aware mapping
rule or a separate authorized shape.

Message mentions contain one entry per direct user, role, here, or all target.
The mapper folds stored recipient rows into these targets and sets
`includes_viewer` from the stored decision. It does not resolve recipients
from current membership or presence during replay. EVT mention rows stay
unchanged.

Public asset processing and deletion events include the owning room and
message IDs. The mapper uses the same retained ownership lookup as event
authorization, including deleted derivatives. Events without a message target
are omitted. The frontend uses these IDs to read each affected message once
and update its loaded timeline, file, and pin rows.

An authorized message-post event carries `body_plaintext` for immediate
display. EVT does not store this field. The frontend inserts a temporary
timeline row from the event ID, actor, time, reply references, and plaintext
body. Values that belong only to the complete message resource start empty.
These values include attachments, link previews, reactions, pin state, thread
counts, thread participants, and the timeline cursor. The server-scoped
[`MessageReconciler`](../../apps/frontend/src/lib/state/server/messageReconciler.ts)
collects affected message IDs for 10 milliseconds, then reads at most 100 IDs
per room with `BatchGetMessages`. It uses the latest received event cursor as
the minimum read boundary. Opaque cursor strings are never sorted.

Each result supplies the same authoritative message to room timelines, open
threads, Files, and pins. Loaded thread roots and echo rows join the same
batch. Related IDs first found in a response use a follow-up batch. Closed
threads do not need a mounted timeline for their files to update. Text-only
posts leave file rows unchanged. File and pin updates preserve loaded pages;
they do not restart the lists. Initial loads, pagination, system-event rows,
and snapshot recovery still use their collection APIs. Message updates do not
replace pagination cursors or imply that a gap in a loaded window is complete.
After a successful thread-read acknowledgement, the root message also uses
this queue. Acknowledgements that arrive during an active read can require a
follow-up batch; they do not refresh the timeline window.

The temporary row uses the connection-scoped user store to resolve its author.
If that store has no profile, the row keeps
its body visible and shows a neutral avatar and a name skeleton. A failed
message read fails reconciliation. An omitted message is removed or tombstoned
through the existing message-deletion rules. Neither case marks the account
as deleted. The shared response replaces a temporary row only if no newer row
change occurred during the read. Account deletion clears
copied author data and the loading state. Deletion fences also apply to late
responses and cached-author fallback.

Message command responses and shared message reads use the same user store as
room directories and the realtime projection. They fetch only missing users and share concurrent reads
for the same user. A profile-change event invalidates that user's summary;
account deletion, projection reset, and store disposal fence pending cache
loads. A missing result from a shared read at a different cursor is retried at
the caller's cursor. Each user request contains at most 100 IDs.


## Exact snapshot and targeted resource reads

`ServerContentView` supplies one exact EVT boundary `E`. The server captures
the complete visible room directory, room-group layout, active calls, public
server profile, and users that these resources reference while the view is at
that boundary. User captures contain encrypted PII, avatar references,
preferences, and roles from the same generation. The server releases the read
barrier before it resolves data-encryption keys, assembles user resources,
encodes protobuf messages, or writes to the WebSocket. Slow key storage or a
KMS cannot stop content-view event application.

One atomic `snapshot` frame contains these canonical `chatto.api.v1` resource
shapes:

| Resource | Protobuf value | Client meaning |
| --- | --- | --- |
| Server profile | `ServerPublicProfile` | Public server profile at `E` |
| Rooms | Repeated `RoomWithViewerState` | Complete visible room directory at `E` |
| Room groups | Repeated `RoomGroup` | Complete visible room-group layout at `E` |
| Users | Repeated `DirectoryMember` | Only the viewer and users referenced by visible snapshot resources |
| Active calls | Repeated `ActiveCall` | Complete visible active-call state at `E` |

The snapshot does not contain the complete user directory. It also excludes
message and thread timelines, search results, files, pins, and other large or
paginated resources. The client reads those resources through ConnectRPC when
it needs them.

The room family includes joined DMs that do not yet contain a message. Each DM
summary says whether it has root-message history. Current `message.read`
authority protects this message-derived value. The bundled client retains an
empty DM for routing but omits it from navigation until it contains a root
message.

Notifications, presence, read markers, account-security state, and process
runtime configuration do not use the EVT boundary owned by
`ServerContentView`. After every `caught_up`, the bundled frontend reads its
required auxiliary state through ConnectRPC before it saves the cursor. These
reads do not redefine the EVT snapshot boundary.

After a durable event, a targeted ConnectRPC request can set
`Chatto-Realtime-Minimum-Cursor` to that event's resume cursor. The common API
interceptor validates the viewer-bound token and waits until the serving
replica includes at least that content boundary. The handler then returns its
normal canonical response. The wait targets exactly the requested EVT sequence
in `ServerContentView`, not the current tails of all projectors. The view
consumes every `evt.>` sequence, including facts that do not change its resources.
A lagging replica waits for at most 10 seconds or the caller's earlier deadline.
A timeout returns `DEADLINE_EXCEEDED` before the handler runs. This is a lower
bound, not a historical read, and does not cover asynchronous effects.

DM threads use the same semantic realtime events and ConnectRPC thread
resources as channel threads. The stream includes DM thread replies, echoes,
root-summary changes, and viewer-state changes without a separate protocol
capability.

Room and thread timelines are not unconditional bootstrap families. The
frontend reloads each mounted timeline at `E` through `RoomService` or
`ThreadService`. A read caused by a later durable event uses that event's cursor
as its minimum boundary. Files and pins retain independent paginated reads
for their collection membership. Changes to their message content use the
shared message queue, whose completion is part of cursor reconciliation.
Search and other lazy data retain their own reads. Canonical events update
resources that the client already uses; they do not open lazy collections.

The bundled frontend gives each cursor-bounded ConnectRPC call a 10-second
deadline. A timeout fails reconciliation and closes the socket without cursor
advance. A new resource reset also starts a new local projection generation.
Late bootstrap, user, resource, and timeline responses from an older generation
cannot change the newer projection.

## Bounded resume

The cursor uses the shared `publiccursor` authenticated-encryption helper.
Its encrypted payload is a 33-byte binary record: a version byte, an 8-byte EVT
sequence, an 8-byte issue time, and a 16-byte SHA-256 prefix of the opaque
stream incarnation. Integers use big-endian order. The version fixes the
15-minute lifetime, so no separate expiry field is needed. The sealed token
is 99 base64url characters. Its encoding is not a public contract.
The purpose and viewer/scope form the authenticated
context. The token expires after 15 minutes. No claim or broker coordinate is
public. Opening the token recovers its sequence directly, without a search.
The 10,000-sequence replay cap does not limit a valid RPC minimum cursor.

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
The `caught_up.recovery` field reports `RESUMED`, `SNAPSHOT`, or `LIVE_ONLY`.
A valid zero-event replay reports `RESUMED`. Outbound events and heartbeats
use `cursor`; only the subscribe request uses `resume_cursor`.

Incremental replay and fallback share one process-local admission guard. Each
replica admits at most eight catch-ups at once and one at a time for each user.
Stale-cursor replay has a per-user burst of three and restores one token every
20 seconds. Cursorless and current-boundary catch-ups use the general burst of
20 and restore one token each second. Metrics expose active, started,
timed-out, and rejected catch-ups.

## Authorization and projection readiness

Privileged-mode changes keep the mounted client state and resume cursor. The
client reconnects and reads current viewer, room, and room-group resources
before it marks catch-up complete. The server cancels authorized work at the session's privilege
deadline and sends a reconnecting `PRIVILEGED_MODE_EXPIRED` close. The client
then reads effective permissions with privileged mode inactive. See
[ADR-096](../adr/ADR-096-session-scoped-privileged-mode.md).

For a valid short gap, the handler subscribes to the process-wide live hub,
captures an EVT cutoff, waits until `ServerContentView` reaches that cutoff
before it reads membership, applicable message-read permissions, interaction
relationships, or compacted state, and performs bounded JetStream point reads
for the sequences after the cursor. It does not create a JetStream consumer. Each
deliverable room, asset, or user fact uses that same content-view readiness
boundary and is converted to a fresh authorized public event. The handler
sends `caught_up` at the cutoff, discards buffered live duplicates through
that sequence, and continues with the hub stream.

Message and asset events require room membership. A viewer also needs
`message.read`, or `message.read-interactions` with a relationship to the
canonical thread root. This rule applies to channel rooms and DMs. Typing
follows the same message-read boundary.

Room visibility and administrative membership facts update the process-wide
visibility cache. Its stable admission boundary includes room creation,
deletion, Universal changes, joins, leaves, member additions, member removals,
bans, and unbans. Facts for a room that a caller never saw are suppressed.

RBAC facts use normal public events in both live delivery and replay. Role
creation, metadata changes, and ordering changes refresh role data without a
full reload. Role catalogue, individual role, and role-member reads wait for
the committed RBAC boundary before reading their local projection. Thus a
follow-up read can use a different replica from the realtime connection.
Assignment and removal events name the user and role so member
lists can update. Role permission events name only the role; direct user
permission events go only to that viewer and contain no private decisions or
scope IDs. Bots also receive a viewer permission event for changes to their
owner's direct decisions or assignments. Role permission changes and deletion
conservatively notify bots because deleted roles no longer retain their former
owner assignments. Cosmetic changes do not reset bots. The client checks
effective authority when its own assignments, a
retained role's permissions, the `everyone` role, or its direct permissions
change. Cursor-bounded resource reads refresh authority in place for every
viewer. Active snapshot queries reauthorize their own scopes; inactive private
snapshots are discarded. The page remains visible and interactive during
the check. Denied or failed reads clear the affected resource, not the server
projection. Permission events do not clear the resume cursor or request a new
WebSocket snapshot.
A replay can
send a viewer's own leave, removal, or ban fact even when current membership
is false. This closing fact removes state that the client could have retained.
Effective membership and message-read permission changes are authorization
boundaries for channel rooms and DMs. An interaction-scoped timeline contains
only related roots. Each message-derived event is authorized against its
canonical thread root. A direct-mention post waits for the Threads projection
before delivery, so that post can establish and use the relationship in order.

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

`live.sync.>` messages use `chatto.core.pubsub.v1.PubSubEvent`. Durable
`live.evt.>` messages use `chatto.core.evt.v1.Event`. The hub decodes each
subject root with its matching envelope. Publishers derive the NATS subject
from a typed user or room scope. Consumers verify that the subject and payload
have the same scope before authorization. Pubsub events have no replay
contract. Durable facts continue through `live.evt.>`, and catch-up resource
reads restore current latest-value state.

A NATS continuity gap or projection-readiness failure quarantines the hub and
closes current sessions. The replica admits a new hub generation only after
NATS resources, projections, and volatile watchers are current. A slow session
that exceeds its queue count or byte limit closes independently.

Known durable room-group, room-layout, and public server-configuration facts
map to dedicated public events. An unknown content-affecting server fact still
quarantines the hub. Key-shredding changes force sessions to rebuild from
current authorized state. Role and permission events leave the connection open;
the client decides when to rebuild its local data. These paths prevent a
client from continuing with state that the server can no longer validate.

Message and asset facts are delivered only when the viewer is a member. A
viewer also needs broad `message.read`, or
`message.read-interactions` with a relationship to the canonical thread root.
The hub and public event mapper both check this boundary.

## Bundled frontend

`ServerStateStore` owns retained `RoomMembersStore` instances for the session.
Room navigation selects an existing store. Public join and leave events update
its membership, and canonical user reads update its profiles. These updates also
apply while the room is not mounted.
The session store also retains presence updates for inactive rooms and rooms
opened later. Catch-up refreshes profiles and presence for retained members.
An event during offset pagination restarts
the membership read with the event's minimum cursor. Recovery resets and room
access loss clear retained membership. Universal-room eligibility changes require
a new authoritative read rather than client-side permission calculations.

[UserStore](../../apps/frontend/src/lib/state/server/users.svelte.ts) stores
public profiles by server, connection scope, and user ID. Directory and timeline
hydration share reads in batches of at most 100 IDs. Realtime updates supersede
pending reads; per-user revisions fence list/detail responses. Deletion markers
prevent old responses from restoring a removed user. Reset rejects pending reads,
and disposal permanently fences the retired owner. Profile expiry timers have
the same lifetime. See [ADR-101](../adr/ADR-101-shared-client-user-profiles.md).
A room's first page and full background load remain separate so
mention completion can use names early and search while loading continues.
Room member state retains membership IDs and resolves profiles from the shared
owner. The quick finder reads that owner directly without starting profile
requests. Server-scoped name and avatar views read the same current profiles.
Three independent presence-filtered scans publish connected members while the
full directory loads. Each status filter also supplies presence for cached
profiles. Per-user change versions prevent these previews from replacing newer
realtime presence. The full scan owns completion and final membership; failed
or late previews cannot block it or restore state after a reset.

The per-server store checks permission events before it changes retained role
assignments. Relevant changes refresh viewer, room, room-group, server-state,
notification, and active-call resources. Unknown viewer role membership also
uses this refresh. The existing projection and cursor remain usable. Unrelated
users' assignments and cosmetic role changes keep the current projection.

Snapshot queries retain their observers and current data while they cancel
older reads. TanStack invalidates the server's snapshot queries before it
refetches active queries, so dependent reads cannot reuse stale snapshots.
Queries that share a dependency also share its replacement request. Failed
permission checks remove cached data; inactive snapshots are discarded without
refetching them. Checks paused while offline hide their cached data and resume
when the client reconnects. Query invalidation also fences late matrix
mutations independently of component disposal. Room membership or message-read
changes clear only the affected plaintext stores and fence their older reads.
Searches keep their input and refresh their results. Fresh route authorization
removes pages whose access was revoked. The shell and other pages remain mounted
and visible. Search and member checks run even when another resource read fails.
Role changes apply before asynchronous checks, so overlapping checks cannot
discard an earlier role change.
Authentication loss and `RESYNC_REQUIRED` still use full privacy cleanup.

An active local call stays connected while private data reloads. Fresh room
permissions then stop only revoked media, or disconnect the call if membership
or `call.join` access was removed. The server independently enforces LiveKit
participant permissions, including when the client cannot finish its reload.
New media actions remain disabled while their permission data is absent.
Catch-up always
loads the viewer's own member record so later role checks have current explicit
assignments, even when no visible room references that viewer.

Each connection has a private-data generation. The ConnectRPC interceptor
rejects older responses before API helpers can publish their data. Reset
handlers run independently; a failed required store cleanup prevents catch-up
from marking the projection ready. The server route hides private children
while the projection is unusable.

Role create/delete completion runs inside the request operation, outside the
route's mutation observer. The application layout owns page-visit tracking.
A reset does not end a visit, but navigation does, including leave and return
to the same URL. A successful obsolete mutation can navigate using submitted
IDs while its response data stays discarded. Connection or session replacement
also prevents old navigation. See ADR-062.

Notification creation hints carry `created_notification_id`, including during
Do Not Disturb and for initially read occurrences. Updates and removals omit it.
The frontend waits for the coalesced notification resource reads, then checks
the retained unread row and its attention level, local read views, Do Not Disturb status,
and per-server sound preferences. This wait adds no RPC and does not consume
cursor-owner failures.
Only newly created unread Important occurrences can trigger sound; Ambient
occurrences remain silent. It groups eligible concurrent creations into one
sound and remembers 256 IDs per server
subscription. Failed reads, missing rows, reset state, and disposed subscriptions
do not play a sound. Periodic reconciliation is silent. Web Push keeps its
server-side policy checks.

The app-icon badge uses Important unread attention across authenticated servers.
It is an unnumbered flag; the window title shows the Important count. Ambient
attention contributes to neither. Push payloads carry `attentionLevel` at the
root and in declarative notification data. The worker sets a flag only for
explicit `important` attention, then asks visible windows to reconcile current
state. Ambient, unknown, and legacy unclassified pushes do not set a badge.
Outgoing push payloads omit numeric app badge values.

Each server store owns a RAM-only
[`ReadViewRegistry`](../../apps/frontend/src/lib/state/server/readViews.svelte.ts).
Visible thread panes register independently and remove their own registration
when hidden or unmounted. Exact room and thread targets permit concurrent views;
a room view does not cover its threads. App focus and visibility gate the shared
attention rule. Notification badges and sound use this rule without changing
server rows or counts. Presentation counts subtract only loaded unread
occurrences covered by a view. Each successful thread read also refreshes its
parent message and followed-thread queries. It refreshes notifications and room
state when the affected room has unread attention, the state is unknown, or an
outstanding read can replace it. These recovery reads use the existing refresh
scheduler without requiring a realtime invalidation.
This read does not replace the open thread's loaded message window.

The bundled frontend selects `SNAPSHOT`. It resets its server projection when
it receives a snapshot and applies all resource families from that one frame.
After every `caught_up`, including a successful resume, it replaces the server
runtime state, viewer, visible rooms, room groups, notifications, and displayed user
presence with cursor-bounded ConnectRPC results. It replaces mounted timelines
only after snapshot fallback because durable replay already repairs timeline
changes. It saves the `caught_up` cursor only after this reconciliation and all
earlier event-triggered resource reads succeed.
Event and heartbeat cursors wait for pending reads without starting this
auxiliary refresh. Thus a replay runs one auxiliary refresh at `caught_up`,
not one refresh per event.
If the socket closes during a snapshot, the client has no resume cursor and
requests a new snapshot.

A replacement snapshot clears private rows and invalidates pending reads, but
does not unmount an already open route. The route stays hidden and inert until
catch-up completes. Room and thread timelines retain only a viewport event ID
and pixel offset across this boundary. Fresh cursor-bounded reads restore that
window; a missing event falls back to the latest window. Access revocation
clears the saved position. Session and explicit resync resets still unmount
private routes. No cached plaintext is used to restore a snapshot.

The projection stores canonical public resources. It does not store
realtime-specific resource copies. Resource invalidation events collect for
10 milliseconds before a ConnectRPC read starts. Adjacent events for the same
family share one read at the latest received cursor. If another event reaches
the same resource family during a read, the frontend runs one follow-up read
at the newest event cursor. Both the collection delay and follow-up reads are
part of cursor reconciliation. Notification invalidations still require an
authoritative notification-list read because their events carry no replacement
notification data. Only occurrence-change hints request that list. Badge hints
request room state, not notifications. A message post requests room state only
when the room is missing from the retained directory; known DM activity is
applied locally. The user-scoped post-commit hint reconciles the poster's read
state and Slow Mode deadline after those updates finish on the server.

A self-authored Badge hint can skip the room read when the room is already
read and Slow Mode is disabled. A room-read hint can also skip an already-read
room. These checks use raw server state, not the attention hidden by an active
view. Unknown state, failed reconciliation, and outstanding reads retain the
refresh. Other actors' Badge hints always refresh rooms because they can either
create or remove attention. Posting can clear older notification occurrences;
their occurrence-change hints still refresh the list. Reconnect reconciliation
is unchanged and repairs missed transient hints. No resources are added to
event payloads, and no new external system receives user data.
The message queue deduplicates pending IDs and serializes batches within each
room. An event that arrives during a read queues another read for its ID and
prevents the older result from being applied. Reset, room-access loss, and
disposal fence outstanding responses. Required author reads are also bounded
to 100 IDs. Both message and author failures prevent cursor advancement.
Remaining timeline-window reads retain each distinct pending anchor, direction,
and minimum cursor. One bounded page cannot replace a read for another anchor.
Identical pending window reads share one request. Cursor
advancement waits for active and queued reads, including reads that started
without a cursor. A failed refresh closes the socket without saving that event
cursor.

After account deletion, the frontend rejects that user's profile in later
user-resource responses before it updates local state or notifies other consumers.
This applies to profile refreshes, DM user reads, and catch-up user batches.
The deletion record stays in memory until the next exact snapshot resets the
projection. Reads from an earlier reset generation cannot update that snapshot.

The DM destination `/chat/[serverId]/dm/[userId]` calls `StartDM` after
navigation. Before it replaces the URL with the canonical room URL, it uses
`ServerStateStore.ensureRoomAvailable` to refresh a missing room through the
same resource pipeline and wait for DM participant hydration. This prevents
the room view from treating a delayed creation event as an unavailable room.
Route cleanup suppresses late navigation. Store disposal and projection resets
invalidate pending reads. Empty DMs remain excluded from sidebar navigation.

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
| `/api/realtime` | One binary `RealtimeSubscribe` message, then `chatto.realtime.v1.RealtimeServerFrame` messages | Bearer credential in `RealtimeSubscribe` or a same-origin cookie; current resource and room visibility apply before mapping | Protocol 4 exact snapshots, authorized public events, and 15-minute bounded resume |

Realtime does not replace `chatto.api.v1`. ConnectRPC remains the public API
for commands, explicit resource reads, pagination, history, search, and
read-your-writes responses.

## Browser push notification cleanup

[`PushNotificationSync`](../../apps/frontend/src/lib/components/PushNotificationSync.svelte)
exists once per authenticated server account. It serializes checks after
notification-store revisions, focus, visibility, network recovery, and the
service worker's visible-app refresh message. Unmount and identity/revision
checks discard stale asynchronous results.

The browser adapter enumerates notifications across service-worker registrations
before a fresh server read. Optional `serverOrigin` and `recipientId` push data
scope each occurrence ID to its owner. The notification store keeps at most
1,024 confirmed local read/delete IDs as a memory-only fast path. Otherwise it
reads the first server page: explicit read rows can close, but absence proves
handling only for a complete page or an exact zero unread count. Optimistic
state and reset placeholders cannot close notifications. Unknown older rows
remain displayed when the response is partial. Checks with no matching browser
notifications make no server request. This path adds no persisted state or
background control push.


Echo post frames resolve body, mentions, and reply attribution from the original
reply after projection readiness. Canonical edit and reaction events refresh
loaded echo rows through their original-message links. No extra durable echo
edit is required.
