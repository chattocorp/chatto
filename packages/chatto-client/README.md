# Chatto client helpers

`@chatto/client` is an internal workspace package for Chatto 0.5 bots and
integrations. It supplies authenticated ConnectRPC JSON requests, message
delivery, complete thread reads, reactions, and typing refresh helpers.
It is not published to npm and has no runtime dependencies.
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

- `rpc<T>` calls a resource service such as `ViewerService/GetViewer` with
  protobuf JSON. `T` is the caller's response type, not runtime validation.
  Use `@chatto/api-types` for generated protocol definitions.
- `createMessage` sends one message and returns the response. It can set
  `inReplyTo` separately from the thread root.
- `postMessage` splits text at 8000 Unicode code points and sends chunks in
  order. A failed chunk stops delivery. Earlier chunks can already have been
  delivered; the helper does not roll them back.
- `readThread` reads all history pages, puts the root first, and removes page
  overlap. It returns textual messages with IDs, authors, and bot/human roles.
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

OAuth login, realtime connections, webhook authentication, deduplication,
conversation routing, and agent behavior remain the host's responsibility.

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
