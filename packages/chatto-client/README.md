# Chatto client

`@chatto/client` is the client for Chatto 0.5 servers. It connects to one or
more servers, keeps their state current through the realtime API, and exposes
that state as reactive stores. It also has the request helpers that bots and
integrations use. It has no dependency on a UI framework. The bundled Chatto
frontend, ChattoBot, and the Runling bot example use it.

It is an internal workspace package and is not published to npm yet. Module
paths can change.

## Clients and connections

A client is an isolated set of servers with their stores, connections,
realtime transports, and background work. Create one client for each
independent host. A process can have several clients, and each client can
connect several servers:

```ts
import { createClient } from '@chatto/client';

const client = createClient();
process.on('SIGTERM', () => client.close());
try {
  const eu = client.connect({ serverUrl: 'https://eu.example', apiKey: euKey });
  const us = client.connect({ serverUrl: 'https://us.example', apiKey: usKey });
  await Promise.all([
    eu.run((ctx) => ctx.reply(`Hello from EU, you said: ${ctx.message.body}`)),
    us.run((ctx) => ctx.reply('Hello from US'))
  ]);
} finally {
  // Stop every connection and all background work.
  client.close();
}
```

`connect()` registers a server with a fixed bearer token, such as a bot API
key. The token is kept only in memory: it is never renewed or written to
device storage.

### Answering messages

`connection.run(handler)` waits until the server accepted the key, also while
the server is unreachable, then calls `handler` for each message addressed to
the viewer (the account of the key):
direct messages, mentions, and replies to the viewer's messages. Messages are
handled in order. The handler receives a context:

```ts
await connection.run(
  async (ctx) => {
    ctx.message; // the addressed message: id, roomId, threadRootId, authorId, body, reasons
    const thread = await ctx.readThread(); // messages with `fromViewer`
    await ctx.withTyping(async () => {
      await ctx.reply(await answer(thread.messages));
    });
    await ctx.addReaction('eyes');
  },
  {
    signal, // stops the loop; without it, the loop stops when the connection closes
    reasons: ['mention', 'direct_message'], // default: all three
    onStatus: (status) => console.log(status.state),
    onError: (error) => console.error(error) // keep running after a failure
  }
);
```

`ctx.signal` aborts when the loop stops or the connection closes; pass it to
your own cancellable work. `ctx.conversationKey` scopes a conversation to the
viewer, room, thread, and sender. `ctx.refreshTyping()` refreshes the typing
indicator once.

`run` resolves when `signal` aborts or the connection closes. It rejects when
the server rejects the key or does not support this client, and, without
`onError`, with the first failure of the handler.

### Events, status, and state

- `connection.consumeEvents({ signal, onEvent, onStatus })` handles every
  realtime event in order. `run` uses it. Up to 1000 received events wait for
  a slow handler; beyond that, they are dropped and a gap is reported.
- `onStatus` receives `connecting`, `ready`, and `reconnecting`. A `ready`
  status with `gap: true` means that events can be missing, for example
  because the server could not resume the stream after a reconnect.
- The connection receives events from its creation, also before `ready()`
  resolves, and keeps them for the first `consumeEvents` or `run` call. A
  later call reports a gap first.
- `onEvent(listener)` and `onReset(listener)` receive events and projection
  resets directly, without a loop.
- `connection.store` is the server's reactive state store, as the frontend
  uses it.

### Readiness and failures

- `ready()` rejects when the server rejects the key, when the server is
  unreachable or fails, or when the server release does not support this
  client. The connection retries in the background; call `ready()` again to
  wait for the next attempt. `run()` waits through these retries.
- `sessionEnded` becomes true when the server rejects or revokes the key.
  `realtimeUnsupported` becomes true when the server does not support this
  client's realtime protocol. In both cases no events arrive, and the event
  loops reject; close the connection.
- A connection's server keeps a persistent WebSocket, also in a client with
  one selected live server.
- `connection.close()` stops the connection's realtime delivery. Requests in
  flight through the connection fail, even when the server applied them, and
  no new requests are sent. When the last connection of a client closes, the
  client stops its timers, so a Node host can exit.

### Requests

A connection and a stateless `Api` have the same request helpers. Each
request has a ten-second timeout and is never retried; a failed write can
still have reached the server.

| Helper                                            | Does                                                   |
| ------------------------------------------------- | ------------------------------------------------------ |
| `getMessage({ roomId, messageId })`               | Reads one message                                      |
| `createMessage(destination, body, { inReplyTo })` | Sends one message                                      |
| `postMessage(destination, body)`                  | Sends text of any length, in parts of 8000 code points |
| `reply(message, body)`                            | Posts in the message's thread with a reference to it   |
| `readThread(location, { after, limit })`          | Reads a thread; `after` reads only newer messages      |
| `refreshTyping(destination)`                      | Refreshes the typing indicator once                    |
| `withTyping(destination, work)`                   | Shows the typing indicator while `work` runs           |
| `addReaction({ roomId, messageId }, emoji)`       | Reacts to a message                                    |
| `addressedMessage(event, { reasons })`            | Recognizes an event as a message to the viewer         |

Every helper takes `{ signal }` as its last argument. Other helpers are
`conversationKey`, `replyDestination`, `withTyping` and `startTyping` for
custom typing updates, and `createDeliveryTracker` for replay filters.

For any other request, `service(Service)` creates a typed Connect client with
the connection's authentication. `@chatto/client/types` has the protocol's
messages and services, and `@chatto/client/types/admin` the administration
API:

```ts
import { UserService } from '@chatto/client/types';

const { user } = await connection.service(UserService).getUser({
  target: { case: 'userId', value: authorId }
});
```

### Stateless requests

`createApi` makes requests without a realtime connection or retained state,
for example in a webhook handler, or for work that can continue after a
connection closes:

```ts
import { createApi } from '@chatto/client';

const api = createApi({ serverUrl, apiKey });
const { viewerId } = await api.ready({ signal }); // read once, then reused
await api.reply(message, 'Done', { signal });
```

It uses Connect JSON, rejects redirects, and loads no stores.

### Debug output

Debug output is off outside browsers. Call `setDebugLogging(true)` to write it
to the console.

## Privacy

Requests contact only the configured servers. Each server receives the host's
IP address, its credentials, and the request data. The client does not log
tokens or message content. Private query data is fenced by connection and
purged at authentication and privacy boundaries (ADR-062).

## Applications with a UI

An application creates one client with device storage and the origin server,
starts its runtime, and reports the server that the user looks at. Only that
server keeps a persistent WebSocket; the others catch up by polling:

```ts
import { createClient } from '@chatto/client/client';

export const client = createClient({
  storage: 'device', // restore and keep the server catalogue and sessions
  originServer: true, // the page's own server uses its cookie session
  liveServers: 'selected',
  voiceCall: (context) => new MyVoiceCall(context)
});

client.start();
client.setActiveServer(serverId);
const store = client.registry.getStore(serverId);
```

A process can have one client with device storage and one for the origin
server. An application that registers a voice-call type must pass its
`voiceCall` factory; `createClient` requires it then.

### Svelte

Import `@chatto/client/svelte` once, before a component reads a store. Svelte
then tracks store reads in components, `$derived`, and `$effect`:

```svelte
<script lang="ts">
  import '@chatto/client/svelte';
  import { client } from '$lib/client';

  let { serverId } = $props();
  const store = $derived(client.registry.getStore(serverId));
</script>

{#each store.navigation.rooms as room (room.id)}
  <p>{room.name}</p>
{/each}
```

Other frameworks can use `setReadHook` and `subscribe` from
`@chatto/client/reactivity` in the same way.

### Voice calls

The client tracks active calls but contains no media implementation. Pass a
`voiceCall` factory to `createClient` and register its type:

```ts
declare module '@chatto/client/register' {
  interface Register {
    voiceCall: MyVoiceCall;
  }
}
```

## Reactivity

The package has a small signal library in `@chatto/client/reactivity`:
`signal`, `computed`, `effect`, `effectRoot`, `batch`, `untrack`,
`subscribe`, `ReactiveMap`, and `ReactiveSet`. Computed values are lazy.
Effects run synchronously when the outermost write ends.

## Development

```sh
mise test-chatto-client   # type checks, lint, and tests
mise build-chatto-client  # compile dist/ for Node hosts
```

`src/types.ts` and `src/types/admin.ts` are generated from
`@chatto/api-types`; `mise codegen-proto` updates them, and the package
check fails when they are out of date.

The package keeps its MIT license.
