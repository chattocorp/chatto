# Chatto client

`@chatto/client` is the client for Chatto 0.5 servers. It connects to one or
more servers, keeps their data current through the realtime API, and exposes
that data as reactive state. It also has the requests and the message loop
that bots and integrations use. It keeps server data only: state of a host's
UI belongs to the host. It has no dependency on a UI framework. The bundled
Chatto frontend, ChattoBot, and the Runling bot example use it.

It is an internal workspace package and is not published to npm yet. Module
paths can change.

## Clients and servers

A client is an isolated set of servers with their realtime transports and
background work. Create one client for each independent host. A process can
have several clients, and each client can connect several servers:

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
  // Close every server and stop all background work.
  client.close();
}
```

`connect()` adds a server with a fixed bearer token, such as a bot API key,
and returns it. The token is kept only in memory: it is never renewed or
written to device storage. `client.server(id)` returns a server that the
client has. Both return the same `Server` type: the server's data, events,
and requests are one object.

### Answering messages

`server.run(handler)` waits until the server accepted the key, also while
the server is unreachable, then calls `handler` for each message addressed to
the viewer (the account of the key):
direct messages, mentions, and replies to the viewer's messages. Messages are
handled in order. The handler receives a context:

```ts
await server.run(
  async (ctx) => {
    ctx.message; // the addressed message: id, roomId, threadRootId, authorId, body, reasons
    const thread = await ctx.readThread(); // messages with `fromViewer`
    await ctx.withTyping(async () => {
      await ctx.reply(await answer(thread.messages));
    });
    await ctx.addReaction('eyes');
  },
  {
    signal, // stops the loop; without it, the loop stops when the server closes
    reasons: ['mention', 'direct_message'], // default: all three
    onStatus: (status) => console.log(status.state),
    onError: (error) => console.error(error) // keep running after a failure
  }
);
```

`ctx.signal` aborts when the loop stops for any reason; pass it to
your own cancellable work. `ctx.conversationKey` scopes a conversation to the
viewer, room, thread, and sender. `ctx.refreshTyping()` refreshes the typing
indicator once.

`run` resolves when `signal` aborts or the server closes. It rejects when
the server rejects the key or does not support this client, and, without
`onError`, with the first failure of the handler.

### Events, status, and state

- `server.consumeEvents({ signal, onEvent, onStatus })` handles every
  realtime event in order. `run` uses it. `onEvent(event, { signal })`
  receives a signal that aborts when the loop stops for any reason, for
  example when the server ends the session. Up to 1000 received events wait
  for a slow handler; beyond that, they are dropped and a gap is reported.
- `onStatus` receives `connecting`, `ready`, and `reconnecting`. A `ready`
  status with `gap: true` means that events can be missing, for example
  because the server could not resume the stream after a reconnect. While
  `run` waits for the server, a `connecting` status with an `error` reports
  each failed attempt, for example an unreachable server.
- A server that `connect` added receives events from its creation, also
  before `ready()` resolves, and keeps them for the first `consumeEvents` or
  `run` call. A later call reports a gap first.
- `onEvent(listener)` and `onSnapshot(listener)` receive events and new
  snapshots directly, without a loop.
- The server's data is reactive: for example `roomList.rooms`,
  `notifications.occurrences`, `projection.users`, and `rooms.messages(id)`
  for a paged, live room timeline.

### Readiness and failures

- `ready()` rejects when the server rejects the key, when the server is
  unreachable or fails, or when the server release does not support this
  client. The server retries in the background; call `ready()` again to
  wait for the next attempt. `run()` waits through these retries.
- `sessionEnded` becomes true when the server rejects or revokes the key.
  `realtimeUnsupported` becomes true when the server does not support this
  client's realtime protocol. In both cases no events arrive, and the event
  loops reject; close the server.
- A server that `connect` added keeps a persistent WebSocket, also in a
  client with one selected live server.
- `server.close()` removes the server from its client and stops its realtime
  delivery. Requests in flight fail, even when the server applied them, and no
  new requests are sent. When the last server that `connect` added closes,
  the client stops its timers, so a Node host can exit.

### Requests

A server and a stateless `Api` have the same request helpers. Each
request has a ten-second timeout and is never retried; a failed write can
still have reached the server.

| Helper                                            | Does                                                      |
| ------------------------------------------------- | --------------------------------------------------------- |
| `getMessage({ roomId, messageId })`               | Reads one message                                         |
| `createMessage(destination, body, { inReplyTo })` | Sends one message; returns its `id`                       |
| `postMessage(destination, body)`                  | Sends text of any length, in parts of 8000 code points    |
| `reply(message, body)`                            | Posts in the message's thread with a reference to it      |
| `readThread(location, { after, limit })`          | Reads a thread; `after` reads only newer messages         |
| `readAttachment({ roomId, attachmentId }, opts)`  | Reads a file; `maxImageSize` resizes images on the server |
| `refreshTyping(destination)`                      | Refreshes the typing indicator once                       |
| `withTyping(destination, work)`                   | Shows the typing indicator while `work` runs              |
| `addReaction({ roomId, messageId }, emoji)`       | Reacts to a message                                       |
| `addressedMessage(event, { reasons })`            | Recognizes an event as a message to the viewer            |

Every helper takes `{ signal }` as its last argument. `postMessage`, `reply`,
and `ctx.reply` return the `ids` of the new messages. Other helpers are
`conversationKey`, `replyDestination`, `withTyping` and `startTyping` for
custom typing updates, and `createDeliveryTracker` for replay filters.

For any other request, `service(Service)` creates a typed Connect client with
the server's authentication. `@chatto/client/types` has the protocol's
messages and services, and `@chatto/client/types/admin` the administration
API:

```ts
import { UserService } from '@chatto/client/types';

const { user } = await server.service(UserService).getUser({
  target: { case: 'userId', value: authorId }
});
```

### Stateless requests

`createApi` makes requests without a realtime connection or retained state,
for example in a webhook handler, or for work that can continue after a
server closes:

```ts
import { createApi } from '@chatto/client';

const api = createApi({ serverUrl, apiKey });
const { viewerId } = await api.ready({ signal }); // read once, then reused
await api.reply(message, 'Done', { signal });
```

It uses Connect JSON, rejects redirects, and loads no stores. Pass
`viewerId` when the host already knows the account of the key, for example
from `server.ready()`; `ready()` then sends no request.

### Debug output

Debug output is off outside browsers. Call `setDebugLogging(true)` to write it
to the console.

## Privacy

Requests contact only the configured servers. Each server receives the host's
IP address, its credentials, and the request data. The client does not log
tokens or message content. The client removes its copies of private data at
authentication, authorization, and privacy boundaries, and reports these
boundaries to the host; see [State of the host](#state-of-the-host).

## Applications with a UI

An application creates one client with device storage and the origin server,
starts its runtime, and reports the server that the user looks at. Only that
server keeps a persistent WebSocket; the others catch up by polling:

```ts
import { createClient } from '@chatto/client';

export const client = createClient({
  storage: 'device', // restore and keep the server catalogue and sessions
  originServer: true, // the page's own server uses its cookie session
  liveServers: 'selected'
});

client.start(); // pair each start() with a stop(), for example on unmount
client.setActiveServer(serverId);
const server = client.server(serverId);
```

A process can have one client with device storage and one for the origin
server.

### State of the host

The client keeps server data. A host keeps its own state, such as sidebar
grouping, search results, call media, or a query cache, next to each server:

- Derive state from the server where possible. Derived state follows the
  server and needs no cleanup.
- A host that copies server data must clear the copy at the server's
  boundary events: `onReset`, `onRoomAccessLost`, `onRoomAccessRestored`,
  `onUserDeleted`, `onAuthorityChanged`, `onPermissionsChanged`,
  `onSessionEnded`, and `onDispose`. `onUpdate` reports every applied event
  and resource. The server emits each event synchronously, after it removed
  its own copies.
- A listener that throws at a privacy boundary fails the private-data
  cleanup: the server then does not report its projection as current. The
  server waits for a promise that an `onPermissionsChanged` listener returns.
- `registry.watchStores(setup)` calls `setup` for each server when the
  registry creates it, before the server receives realtime data.

### Svelte

Import `@chatto/client/svelte` once, before a component reads a server.
Svelte then tracks reads in components, `$derived`, and `$effect`:

```svelte
<script lang="ts">
  import '@chatto/client/svelte';
  import { client } from '$lib/client';

  let { serverId } = $props();
  const server = $derived(client.server(serverId));
</script>

{#each server?.roomList.rooms ?? [] as room (room.id)}
  <p>{room.name}</p>
{/each}
```

Other frameworks can use `setReadHook` and `subscribe` from
`@chatto/client/reactivity` in the same way.

### Voice calls

A server tracks its active calls in `projection.activeCalls`. The client
contains no media implementation. A host that joins calls implements the
media, reads call permissions from the projected rooms, and leaves a call at
`onRoomAccessLost` and `onDispose`.

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
