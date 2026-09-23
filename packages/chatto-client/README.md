# Chatto client helpers

`@chatto/client` is an internal workspace package for Chatto 0.5
integrations. It supplies authenticated ConnectRPC JSON requests, message
delivery, complete thread reads, reactions, typing refresh helpers, and realtime events.
It is not published to npm. Realtime uses `@chatto/api-types` and its protobuf runtime.
The host must provide Fetch and `AbortSignal.any`/`AbortSignal.timeout`.

```ts
import { createChattoClient } from "@chatto/client";

const client = createChattoClient({ serverUrl, apiKey: botApiKey });
await client.postMessage({ roomId, threadRootId }, "Hello from the bot", signal);
```

The host supplies credentials and can inject `fetch`. The package does not
read `.env`, start a server, or depend on Runling. Requests contact only the
configured Chatto server, which receives the caller's IP address, bearer token,
and request data. Redirects are rejected. Transport errors omit response bodies,
credentials, and URLs. The package does not log requests or responses.

## Behavior

- `getViewer({ signal })` returns `{ id }` for the authenticated viewer. Missing
  identity is an error. The helper does not cache identity across credential changes.
- `getMessage({ roomId, messageId, signal })` returns a normalized message with
  `id`, `roomId`, `authorId`, and optional `body`, `threadRootId`, and `inReplyTo`.
  It is a projection for integrations, not a complete renderable message.
  Missing or mismatched message identity returns `undefined`; RPC failures reject.
- `rpc<T>` calls a resource service such as `ViewerService/GetViewer` with
  protobuf JSON. `T` is the caller's response type, not runtime validation.
  Use `@chatto/api-types` for generated protocol definitions.
- `createMessage` sends one message and returns the response. It can set
  `inReplyTo` separately from the thread root.
- `postMessage` splits text at 8000 Unicode code points and sends chunks in
  order. A failed chunk stops delivery. Earlier chunks can already have been
  delivered; the helper does not roll them back. Set `destination.inReplyTo` to
  associate every chunk with its prompting message. The thread root stays separate.
  `createMessage` also accepts this destination field; its explicit reply argument
  takes precedence. Existing destinations need no changes.
- `readThread({ roomId, threadRootId }, signal)` reads all history pages, puts
  the root first, and removes page overlap. It returns textual messages with
  IDs and authors, without bot-specific roles.
  It rejects missing pages and repeated or missing pagination cursors.
- `addReaction` targets a message event. The host chooses the emoji.
- `refreshTyping` makes one presence request. `withTyping` refreshes during
  work without overlapping requests and aborts the current refresh when work
  ends. Typing failures do not fail the primary work.
- `startTyping` waits for the initial update and returns a stop function. It
  stops future refreshes but does not cancel an update already in flight.
  The host must bound that update with a timeout.

Requests have a ten-second timeout. A complete thread read has a thirty-second
total timeout. Caller cancellation also reaches the transport. The client never
retries requests: a failed connection can leave delivery uncertain.

Bot conventions live in [`@chatto/bot-client`](../chatto-bot-client/README.md).
This includes addressing recognition, bot-relative thread roles, reply context,
conversation keys, and process-local deduplication. OAuth login, webhook
authentication, workflow routing, and agent behavior remain host responsibilities.

Migration: `client.addressedMessage` and its addressing types moved to the bot
package. Thread reads now take `{ roomId, threadRootId }` instead of a webhook
delivery and no longer return bot/human roles. Use the bot adapter when needed.

## Realtime events

The host must supply WebSocket support. Node 22.19 and later provide it. Tests
can pass `webSocket` to `createChattoClient`; this factory must reject redirects.

```ts
const checkpoint = {}; // Keep only in memory, for this server and API key.
await client.consumeRealtime({
  signal,
  checkpoint,
  async onEvent(event) {
    if (event.event.case === "messagePosted") await acceptMessage(event);
  },
  onStatus(status) {
    if (status.state === "ready" && status.gap) reportMissedMessages();
  },
});
```

`acceptMessage` and `reportMissedMessages` represent host functions. The client
connects to `/api/realtime` with protocol v4 and sends the API key in the first
binary frame. Only the configured server receives the caller's IP address and key.

Events arrive in order. Resolve `onEvent` after accepting the delivery, for
example after registering a run or inserting a message into an inbox. The
client then advances the checkpoint. It does not wait for downstream work.
Unknown semantic events can be skipped. Invalid or unknown top-level frames
stop consumption without advancing past them.

Transient failures reconnect with backoff and the last accepted cursor.
Server retry delays are respected. The queue is limited to 256 pending frames
and 8 MiB; overflow reconnects after the current handler settles. Set
`maxPendingFrames` to change the frame limit. Handlers must settle promptly.
Cancellation closes the transport and waits for any current handler.

A new process starts live. An expired cursor or unavailable replay also starts
live. A `ready` status with `gap: true` reports possible missed messages after
a prior connection or resume attempt. Cursors expire after 15 minutes. There
is no automatic history fetch, durable inbox, or exactly-once delivery.
Handler failures and terminal protocol errors reject consumption with a safe
error. Resolve the cause before restarting. Failed handlers retain the prior cursor.

## Development

From the monorepo root:

```sh
mise build-chatto-client
mise test-chatto-client
mise check-runling
mise test-runling
mise test-runling-bot
```

Runling's examples use this package as a development dependency; the published
Runling runtime has no Chatto dependency. Root build and verification tasks
build this package before consumers resolve its exports.

## License

MIT, preserving the license of the helpers extracted from Runling.
See [LICENSE](LICENSE) and [ADR-100](../../docs/adr/ADR-100-shared-chatto-integration-client.md).
