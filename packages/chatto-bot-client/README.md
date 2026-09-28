# Chatto bot client

`@chatto/bot-client` is a private MIT workspace package. It adds bot conventions
to [`@chatto/client`](../chatto-client/README.md), without a Runling dependency.
It does not load environment variables or own a workflow.

## Receive and reply

```ts
import { connectChatto } from '@chatto/client';
import { createBotClient, createDeliveryTracker } from '@chatto/bot-client';

const chatto = connectChatto({ serverUrl, apiKey });
try {
  const bot = await createBotClient(chatto, { signal });
  const deliveries = createDeliveryTracker();
  await bot.consumeEvents({
    signal,
    async onEvent(event) {
      if (deliveries.has(event.id)) return;
      const message = await bot.addressedMessage(event, { signal });
      if (!message) return;
      await bot.reply(message, 'Hello!', signal);
      deliveries.accept(event.id);
    }
  });
} finally {
  chatto.close();
}
```

The host supplies `serverUrl`, `apiKey`, and `signal`. `createBotClient` waits
until the server accepted the key and resolves the bot's identity once.
Requests contact only the configured Chatto server. That server receives the
host IP address, the key, and request data. The bot package adds no external
service or logging.

`consumeEvents` handles events in order: the next event waits until
`onEvent` resolves. At most 1000 received events wait; when a slow handler
falls further behind, the waiting events are dropped and a gap is reported. It
resolves when `signal` aborts. It rejects when `onEvent` or `onStatus` throws,
when the server ends the session (for example after the API key is revoked),
or when the connection closes. `onStatus` reports `connecting`, `reconnecting`, and `ready`. A
`ready` status with `gap: true` means that the server could not resume the
stream, so events can be missing.

For short-lived work without a realtime connection, such as a webhook handler,
use `createBotApi(createChattoApi({ serverUrl, apiKey }), botId)` with
`createChattoApi` from `@chatto/client/apiClient`. It has the same request
helpers as the bot client.

## Helpers

- `bot.postMessage(destination, body, signal)` splits text at 8000 Unicode
  code points and sends the chunks in order. A failed chunk stops delivery;
  earlier chunks can already have been delivered. `bot.createMessage` sends
  one message. Neither retries a request.
- `bot.readThread(location, signal)` reads all history pages within 30
  seconds and puts the root first. It rejects missing pages and pagination
  that does not advance. `bot.readBotThread(location, signal)` adds bot/human
  roles. Here, `human` means any author other than this bot; it is not an
  account-type check.
- `bot.refreshTyping(destination, signal)` refreshes the typing indicator once.
  `withTyping` and `startTyping` refresh it during work without blocking it.
- `bot.addReaction(roomId, messageEventId, emoji, signal)` reacts to a message.
- `bot.addressedMessage(event, { signal, reasons })` recognizes textual direct
  messages, viewer mentions, and verified replies to the bot. All three are
  enabled by default. Use `reasons: ["mention"]` for a mentions-only policy.
  Self-authored messages, unrelated events, and unavailable text are ignored.
  Unmentioned replies require a lookup to verify author, room, and thread.
  Lookup errors and cancellation propagate. Apply sender restrictions first.
- `bot.reply(message, body, signal)` posts in the original thread and sets
  `inReplyTo` to the prompting message. `replyDestination(message)` provides
  the same destination for other operations.
- `bot.conversationKey(message)` scopes a conversation to the bot, room,
  thread, and sender. Hosts can use a different key. Store keys separately for
  each server. `conversationKey(viewerId, message)` is the standalone form.

Each request has a ten-second timeout. A request is not sent after `signal`
aborts.

## Acceptance and reloads

`createDeliveryTracker()` stores accepted IDs for 24 hours by default. Call
`has(id)` to check replay and `accept(id)` only after successful handling. For
background work, accept after inbox insertion or successful run registration,
not after workflow completion. Failed registration must remain retryable.
The tracker does not reserve concurrent deliveries; the host owns serialization.

Retain the tracker, or its optional `accepted` map of IDs to expiry times, across
reloads for the same server and bot identity. Use separate storage for a new
identity. Conversation state, cancellation, and reload handoff remain
application concerns.

A new connection starts from a new realtime snapshot. Events that arrive while
no connection is open are not replayed.

This is process-local deduplication, not a durable inbox or an exactly-once
guarantee. A failed write can have reached the server. A process restart loses
acceptance state, and recovery gaps do not trigger automatic history reads.

## Migration

The helpers moved from the old `@chatto/client` integration client (ADR-110):

- Replace `createChattoClient({ serverUrl, apiKey })` with `connectChatto` and
  `createBotClient`, or with `createChattoApi` and `createBotApi`.
- Replace `client.consumeRealtime({ checkpoint, onEvent, onStatus })` with
  `bot.consumeEvents({ signal, onEvent, onStatus })`. There is no checkpoint.
- `postMessage`, `createMessage`, `readThread`, `refreshTyping`,
  `addReaction`, `getMessage`, `withTyping`, and `startTyping` are now bot
  client functions and exports of this package. `createMessage` returns
  `{ id }`.
