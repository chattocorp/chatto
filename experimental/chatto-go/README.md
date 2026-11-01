# Chatto Go SDK experiment

This standalone Go module contains the Chatto connection code used by
[Gobotto](../gobotto/README.md). It is the starting point for a future official
Go SDK. [Chatty](../chatty/README.md), an experimental human terminal client,
also uses this module. Its API is incomplete, unstable, and not yet published
separately.

The package name is `chatto`. It owns:

- Server URL validation and API-token authentication.
- Connect JSON calls for viewer identity, thread reads, replies, and reactions.
- Realtime protocol v4 WebSocket subscriptions and private protobuf types.
- Verified mention and DM delivery, self-message and echo filtering, and bounded
  duplicate protection for each connection.
- Message thread-root resolution and UUID generation for one-time sends.

There is no dependency on Gobotto, OpenRouter, environment-variable names,
agent sessions, or server-internal Go packages. Callers supply configuration,
callbacks, and cancellation contexts. The package does not write logs.

The library has three boundaries: stateless `Client` requests, optional `Server`
connection stores, and explicit convenience projections and bot observers. Hosts
own display filters, image sizes, history retention, and credential persistence.
Room groups and notification counts remain available as protocol resources;
applications do not have to use them to build navigation or notifications.

## API

Create a client with `chatto.New(serverOrigin, apiToken)`. Use `Viewer` to
read the authenticated account's ID, login, and display name through
`ViewerService/GetViewer`. `ViewerID` remains available when only the ID is needed.
Pass the profile's ID to `Pings`. Its callback
receives DMs and verified mentions. `Message.ThreadRootID()` resolves the
conversation root. Use `Thread`, `Reply`, and `React` for messaging operations.

Use `RefreshTypingIndicator(ctx, roomID, threadRootID)` for live typing activity.
An empty thread root targets the room timeline. Refresh at most once every two
seconds and stop when work ends. Receivers clear typing when the account posts
in that location or after six seconds without a refresh. There is no stop call.

Use `ObserveMessages` when you also need unaddressed context and invalidations.
Its `MessageEvent.Addressed` flag distinguishes DMs/verified mentions from other
visible messages. Self-posts and echoes are omitted. `InvalidatedRoom` signals
edits, retractions, room deletion, or the viewer leaving a room. `InvalidateAll`
signals viewer-permission changes or account deletion. Discard or reconcile
retained context before further use. Keep callbacks short and queue model work
outside the realtime reader.

For edits and retractions, `InvalidatedMessage` supplies the changed message ID,
and `InvalidationReason` identifies the event type. Consumers can limit resets to
conversations that contain that message in retained or pending context. If the
target is missing or context membership is not yet known, reconcile conservatively.
Existing consumers that use only `InvalidatedRoom` remain safe but invalidate
more context than necessary. No server upgrade is required.

`Pings` blocks until cancellation, a connection failure, or a callback error.
Callbacks run sequentially. Startup is live-only; there is no automatic
reconnect or replay. Thread reads return the root and up to 50 recent replies,
with opaque cursors for later pages. Messages include attachment metadata;
consumers that send context to models must select their own permitted fields.

Use `ObserveResumable` for automatic reconnect and bounded replay. Supply a saved
cursor in `RecoveryOptions.Cursor`. Its `Checkpoint` callback must atomically
save the supplied cursor and any non-nil event to a durable inbox. The SDK calls
it before the message handler. A nil event updates only the cursor; an empty
cursor clears it. Drain saved inbox entries before opening the subscription.
Callbacks run in order, and callback errors stop recovery. Event IDs support
deduplication across connections; the SDK's own duplicate filter is per connection.

Reconnect uses exponential backoff from one to 30 seconds, plus up to 50 percent
jitter, and honors longer server-requested delays. Cancellation interrupts the
wait. Authentication rejection and non-retryable closes stop the subscription.
Heartbeats update the cursor even when there are no messages. Chatto cursors
expire after 15 minutes. A failed resume emits `MessageEvent.RecoveryGap` before
the ready callback; refresh retained context and report the possible message gap.
Do not start queued model work until the ready callback confirms replay completion.
The `Status` callback reports disconnects so callers can pause queued work again.

`Reply` accepts an optional idempotency UUID. Supply and retain a UUID if you
need an exact retry after an uncertain result. With an empty key, the SDK creates
a fresh UUID and sends once. Repeating that call creates a new send attempt.
The SDK does not automatically retry requests.

## Integration APIs

### Stateful server connection

Use `Connect` for a server with shared stores, initial synchronization, and
automatic reconnect. The existing `Client` remains the stateless request API.
Existing bot observers do not change behavior.

```go
client, err := chatto.New(origin, token)
if err != nil {
    return err
}
server := chatto.Connect(ctx, chatto.ConnectionOptions{
    Client: func(context.Context, bool) (*chatto.Client, error) {
        return client, nil // Fixed API token. Human hosts renew their session here.
    },
})
defer server.Close()
if err := server.WaitReady(ctx); err != nil {
    return err
}
state := server.State()
// state.Profile, Viewer, Rooms, Groups, Users, and Timelines are detached copies.
updates, unsubscribe := server.Subscribe()
defer unsubscribe()
_ = updates // Read a signal, then read the latest state.
```

The provider must keep the same server origin and account. It can run concurrently
with resource reads. Its `renew` argument is true when the server requires session
rotation. Human hosts must save refresh request identity and replacement credentials
as described below. A provider error stops the connection.

The connection requests the protocol-v4 snapshot fallback. It commits the server
profile, visible rooms, ordered room groups, and referenced users only after
`caught_up`, together with any catch-up events. Missing DM participant profiles
are resolved once during snapshot initialization. Large histories load on demand
with `LoadTimeline(ctx, room, root, before)`. An empty root selects the room timeline;
an empty cursor reuses a loaded timeline. Older pages merge into the same store.
`LoadTimelineWithOptions` also accepts an `after` cursor and a page limit through
`TimelineOptions`. Zero limit uses the server default. Room directory entries
retain archived rooms. Applications choose which entries to display. Timeline
access does not require a directory entry; the API decides access.

Realtime posts, edits, retractions, room metadata/membership changes, read-state
changes, profile changes, and room-layout changes update the stores. Events that
only identify a resource trigger a targeted read with the event's minimum cursor.
Message bodies come from the message resource, not the plaintext event preview.
Set `ConnectionOptions.IncludeNotifications` to load authoritative per-room
notification counts into `ServerState.Notifications`. Ordinary attention is
`Unread - Important`. Counts refresh on notification events and reconnect;
expired counts refresh on the next realtime heartbeat. Unsupported count APIs
leave the map empty without blocking messaging. This option defaults to false.
`ListNotifications` reads paginated occurrences without choosing presentation or
attention filters. `NotificationOccurrence.MessageTarget` resolves known message
signals and leaves unknown signals and invitations without an invented message
destination. Hosts choose which occurrence to navigate to and whether to mark it read.
This projection does not yet expose reactions, assets, typing, presence, calls,
roles, or full notification occurrences. Permission and account changes conservatively
clear private state and request a replacement snapshot.

Reconnect retains the last applied event or heartbeat cursor, with exponential
backoff, jitter, and server retry guidance. Accepted replay retains loaded history.
Snapshot fallback replaces the stores and discards old timelines. Failed handshakes
do not publish partial snapshots. In-flight resource reads cannot restore data
across a reset. `ServerState.Generation` identifies store replacement; `Ready`
indicates that committed data is available, also during a brief disconnection.
Terminal failure and `Close` clear the stores. Cursors and histories are in memory
only; a process restart requests a fresh snapshot. Use `ReleaseTimeline(key)` to
remove cached history. It also prevents an in-flight load from restoring that
history. A later explicit load can populate it again. Hosts choose when to release
histories; reset and close discard all stores.

`Subscribe` supplies an independent coalesced channel and an idempotent cancel
function. It sends an initial signal. Cancel each subscription when finished,
including after the connection closes. Cancellation closes its channel.
The older `Updates` channel remains available for one consumer. Neither channel
is an event log. Read current state after a signal. `ReadState(keys...)` copies
metadata and only the selected cached timelines, atomically. With no keys, it
copies no histories. `State` copies all stores. Chatty uses focused reads and an
independent subscription without polling the directory.

### Session credentials

`LoginBrowser` signs in through the server's browser flow with S256 PKCE and a
temporary loopback callback. It uses the existing `chatto://loopback` client;
the server must enable `auth.loopback_client_enabled`. The callback passed to
this function opens or displays the authorization URL. Never log that URL.
`LoginPassword` supports servers with direct login enabled. Neither function
stores credentials or opens a browser itself.

Both return `HumanSession`. Save it in private storage and obtain an immutable
API client with `session.Client()`. Before access-token expiry, call
`PrepareRefresh`, persist the session, then call `Refresh`. Persist the returned
session before using its credentials. Keep the old prepared session on failure;
an uncertain retry must use the same refresh token and request ID. `Logout`
revokes the session. The caller owns synchronization and credential persistence.

`ListRooms` exposes caller-selected offset and limit; `Rooms` is a convenience
wrapper with a 100-room page. `RoomGroups` returns the finite ordered
channel-group snapshot, including room and sidebar-link items. DMs remain in
`Rooms`. `Timeline` reads a room or thread and
accepts a server-owned `before` cursor for older messages. It returns message
posts and included author profiles; lifecycle events are omitted.
`TimelineWithOptions` accepts a page limit and either cursor direction.
`TimelineEvents` returns all events and included resources as raw JSON, including
types not yet modeled by this SDK. `Profiles`
resolves batches of up to 100 public users. `NewMessageKey` supplies an outgoing
send identity; `Reply` also sends room roots when the thread root is empty.

`TimelineAround` reads a window around an exact message in a room or thread.
`Server.LoadTimelineAround` replaces that timeline's cached window with the result;
it does not combine disjoint history. Both use the existing around-event APIs.
When `HasNewer` is true, live posts outside the window do not append across the
unloaded gap. Existing messages still receive edits. Page forward with `After`,
or release the timeline and load it again to return to latest history.

Messages include attachment metadata. `GetAsset` checks current room access and
returns fresh metadata and signed URLs for any file type. Pass an optional
`ImageTransform` to request a thumbnail with caller-selected dimensions and fit.
`DownloadAsset` fetches fresh authorization and encoded bytes. Its
`AssetDownloadOptions.MaxBytes` must be positive; the caller supplies a deadline
through the context. A nil thumbnail selects the original file.
It accepts only URLs on the connected server's origin,
does not follow redirects, and sends no bearer token with the signed download.
It returns encoded bytes; callers must bound decoded dimensions before they
allocate image pixels. Signed asset URLs are credentials: do not log or persist
them. Downloads do not require a separate asset host. Decoded-file limits remain
the caller's responsibility. The experimental `ImagePreview` and `ImageOriginal`
helpers were replaced by `DownloadAsset`; Chatty now owns its thumbnail size,
download limits, and timeout.

`ObserveTimeline` reconnects and observes visible posts, including self-posts,
echoes, and messages without text. It also supplies the same history and access
invalidations as `ObserveResumable`. Hydrate events through resource reads: the
event body is only a plaintext preview. This is not a complete room/profile or
reaction projection. Existing bot observers keep their suppression behavior.

## Development

From this directory:

```sh
mise check
mise codegen
```

The module stays outside the root Go workspace. Its `mise.toml` sets
`GOWORK=off`; standalone Go commands need the same setting.

`internal/wire/realtime.proto` is a wire-compatible subset of the public schemas
in `proto/chatto/realtime/v1/` and the room-kind enum in
`proto/chatto/api/v1/rooms.proto`. Keep field numbers and types aligned with
those sources. `mise codegen` regenerates the private Go types.

Gobotto uses a local module replacement for this directory. To extract the SDK,
move this module, select its published module path, and update that dependency.
The current repository license applies; publication and license policy are
separate decisions.
