# Chatto client

`@chatto/client` is the client for Chatto 0.5 servers. It connects to one or
more servers, keeps their state current through the realtime API, and exposes
that state as reactive stores. It has no dependency on a UI framework. The
bundled Chatto frontend and ChattoBot use it.

It is an internal workspace package and is not published to npm yet. Module
paths can change.

## Headless hosts

`connectChatto` connects one server with a fixed bearer token, such as a bot
API key. It uses the same stores, recovery, and realtime transport as the
frontend.

```ts
import { connectChatto } from '@chatto/client';

const chatto = connectChatto({ serverUrl, apiKey });
const { viewerId } = await chatto.ready({ signal });
chatto.onEvent((event) => {
  if (event.event.case === 'messagePosted') handle(event);
});
const rooms = [...chatto.store.projection.rooms.values()];
// Close the connection when the host stops.
chatto.close();
```

- A process can have one open connection. Close it before you connect again.
- `chatto.service(Service)` creates a typed Connect client with the
  connection's authentication.
- `onEvent` listeners run in order after the store applied an event. They must
  not throw. `onReset` reports projection resets. `gap: true` means that a
  new snapshot replaced a stream that the server could not resume, so events
  can be missing.
- `close()` stops realtime delivery and all timers, and rejects a pending
  `ready()`. Requests in flight through the connection fail, even when the
  server applied them. Use `createChattoApi` for work that can continue
  after the connection closes.
- The token is kept only in memory: it is never renewed or written to device
  storage. When the server rejects it, `ready()` rejects and `sessionEnded`
  becomes true. Close the connection then.
- Debug output is off outside browsers. Call `setDebugLogging(true)` to
  write it to the console.
- When the server is unreachable or fails, `ready()` rejects and the
  connection retries in the background with a backoff. Call `ready()` again
  to wait for the next attempt.

`createChattoApi` makes stateless requests without a realtime connection, for
example in a webhook handler. Import it from `@chatto/client/apiClient`, which
does not load the stores:

```ts
import { createChattoApi } from '@chatto/client/apiClient';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';

const api = createChattoApi({ serverUrl, apiKey });
const viewer = await api.service(ViewerService).getViewer({}, { signal });
```

It uses Connect JSON, rejects redirects, and never retries requests.

Bot conventions, such as message splitting, thread reads, and addressing, are
in [`@chatto/bot-client`](../chatto-bot-client/README.md).

## Privacy

Requests contact only the configured servers. Each server receives the host's
IP address, its credentials, and the request data. The client does not log
tokens or message content. Private query data is fenced by connection and
purged at authentication and privacy boundaries (ADR-062).

## Applications with a UI

Applications use the module entries directly, for example
`@chatto/client/server/registry` and `@chatto/client/server/runtime`. They
start the client runtime once and report the server that the user looks at:

```ts
import { startClientRuntime } from '@chatto/client/server/runtime';

const runtime = startClientRuntime();
runtime.setActiveServer(serverId);
```

### Svelte

Import `@chatto/client/svelte` once, before a component reads a store. Svelte
then tracks store reads in components, `$derived`, and `$effect`:

```svelte
<script lang="ts">
  import '@chatto/client/svelte';
  import { serverRegistry } from '@chatto/client/server/registry';

  let { serverId } = $props();
  const store = $derived(serverRegistry.getStore(serverId));
</script>

{#each store.navigation.rooms as room (room.id)}
  <p>{room.name}</p>
{/each}
```

Other frameworks can use `setReadHook` and `subscribe` from
`@chatto/client/reactivity` in the same way.

### Voice calls

The client tracks active calls but contains no media implementation. Install
one with `setVoiceCallFactory` from `@chatto/client/server/voiceCall` and
register its type:

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
mise test-chatto-client   # type checks and tests
mise build-chatto-client  # compile dist/ for Node hosts
```

The package keeps its MIT license.
