# Chatto Go SDK experiment

This standalone Go module contains the Chatto connection code used by
[Gobotto](../gobotto/README.md). It is the starting point for a future official
Go SDK. Its API is incomplete, unstable, and not yet published separately.

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
with opaque cursors for later pages. Attachments are not decoded.

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
