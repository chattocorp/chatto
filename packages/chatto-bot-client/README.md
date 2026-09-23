# Chatto bot client

`@chatto/bot-client` is a private MIT workspace package. It adds bot conventions
to [`@chatto/client`](../chatto-client/README.md), without a Runling dependency.
It does not start a connection, load environment variables, or own a workflow.

## Receive and reply

```ts
import { createChattoClient } from "@chatto/client";
import { createBotClient, createDeliveryTracker } from "@chatto/bot-client";

const client = createChattoClient({ serverUrl, apiKey });
const bot = await createBotClient(client, { signal });
const deliveries = createDeliveryTracker();
const checkpoint = {};

await client.consumeRealtime({
  signal,
  checkpoint,
  async onEvent(event) {
    if (deliveries.has(event.id)) return;
    const message = await bot.addressedMessage(event, { signal });
    if (!message) return;
    await bot.reply(message, "Hello!", signal);
    deliveries.accept(event.id);
  },
});
```

The host supplies `serverUrl`, `apiKey`, and `signal`. The adapter resolves the
viewer once. Create a new adapter when the client or credentials change.
Identity and reply checks contact only the configured Chatto server through the
client. That server receives the host IP address, credentials, and request data.
The bot package adds no external service or logging.

## Helpers

- `bot.addressedMessage(event, { signal, reasons })` recognizes textual direct
  messages, viewer mentions, and verified replies to the bot. All three are
  enabled by default. Use `reasons: ["mention"]` for a mentions-only policy.
  Self-authored messages, unrelated events, and unavailable text are ignored.
  Unmentioned replies require a lookup to verify author, room, and thread.
  Lookup errors and cancellation propagate. Apply sender restrictions first.
- `bot.reply(message, body, signal)` posts in the original thread and sets
  `inReplyTo` to the prompting message. `replyDestination(message)` provides
  the same destination for other client operations.
- `bot.conversationKey(message)` scopes a conversation to the bot, room,
  thread, and sender. Hosts can use a different key. Store keys separately for
  each server. `conversationKey(viewerId, message)` is the standalone form.
- `bot.readThread(location, signal)` adds bot/human roles to thread text.
  Here, `human` means any author other than this bot; it is not an account-type
  check. `readBotThread(client, viewerId, location, signal)` accepts a known identity.
- `addressedMessage(client, event, { viewerId, signal, reasons })` is the
  standalone addressing helper. Obtain `viewerId` from that client's `getViewer()`.

## Acceptance and reloads

`createDeliveryTracker()` stores accepted IDs for 24 hours by default. Call
`has(id)` to check replay and `accept(id)` only after successful handling. For
background work, accept after inbox insertion or successful run registration,
not after workflow completion. Failed registration must remain retryable.
The tracker does not reserve concurrent deliveries; the host owns serialization.

Retain the tracker, or its optional `accepted` map of IDs to expiry times, across
reloads for the same server and bot identity. Use separate storage for a new
identity. Retain the realtime checkpoint only for the same server and credentials.
Conversation state, cancellation, and reload handoff remain application concerns.

This is process-local deduplication, not a durable inbox or an exactly-once
guarantee. A failed write can have reached the server. A process restart loses
acceptance state, and recovery gaps do not trigger automatic history reads.

## Migration from the client package

`client.addressedMessage` moved here. Create a bot adapter or call the standalone
helper. Import `AddressedMessage` and `AddressingReason` from this package.
`client.readThread` now takes `{ roomId, threadRootId }` and returns messages
without bot/human roles. Use `bot.readThread` when those roles are required.

## Development

Run `mise build-chatto-bot-client` and `mise test-chatto-bot-client` from the
repository root. Tests use local transport doubles and make no model calls.

See [ADR-100](../../docs/adr/ADR-100-shared-chatto-integration-client.md)
for the package boundary and [LICENSE](LICENSE) for the MIT license.
