# ADR-111: Move the Client State Layer into `@chatto/client`

**Date:** 2026-09-28

## Status

Partially superseded by [ADR-112](ADR-112-keep-the-server-catalogue-in-the-frontend.md),
which keeps the server catalogue in the frontend. Supersedes [ADR-100](ADR-100-shared-chatto-integration-client.md).
Amends [ADR-010](ADR-010-svelte5-reactive-cache-whitelisting.md),
[ADR-025](ADR-025-multi-instance-client-architecture.md), and
[ADR-062](ADR-062-tanstack-query-for-snapshot-reads.md).

## Context

The bundled frontend kept its complete client in `apps/frontend/src/lib`: the
ConnectRPC facades, sessions and token renewal, the server registry, the
realtime transport, the resumable projection, and the server and room stores.
This code used Svelte runes and Svelte collections. Other hosts could not use
it.

ADR-100 added `@chatto/client` as a small, separate client for bots. That
client sent JSON requests and read realtime events without the frontend's
projection, recovery, and privacy fences. So Chatto had two clients with
different behavior, and bots could not use the tested frontend client.

We want one client. The frontend, ChattoBot, and third-party bots and
frontends must use the same code, and tests must cover this code without a
UI framework. A host must be able to run several independent clients, for
example several bots that connect to different servers.

The client must serve integrations. State that only one host's UI uses, such
as sidebar grouping, search sessions, or call media, must not be part of it.

## Decision

`@chatto/client` (`packages/chatto-client/`) contains the Chatto client. The
package has no dependency on the frontend. Only the `@chatto/client/svelte`
adapter uses Svelte, as an optional peer dependency. The old integration
helpers and the separate `@chatto/bot-client` package are removed: a bot uses
the same client as every other host.

### Package contents

The package owns this client behavior:

- Client instances (`client`): `createClient()` creates an isolated client
  with its own registry, connection manager, realtime transports, and
  runtime.
- Servers (`server/server`): one type for every server of a client, with its
  state, its events, its requests, and the message loop for bots. Stateless
  requests (`api`) share the request helpers (`messaging/`).
- Boundary events of each server (`server/storeEvents`); see
  [Server data and host state](#server-data-and-host-state).
- Re-exports of the protocol messages and services (`types`,
  `types/admin`), generated from `@chatto/api-types`.
- The ConnectRPC facades, the transport interceptors, and the privacy fences
  (`api/`).
- Sessions, bearer renewal, and origin cookie sessions (`auth/`, `server/registry`).
- Server connections and the realtime transport (`server/serverConnection`,
  `server/realtimeTransport`, `realtime/eventBus`).
- The realtime projection and the server data: rooms and room groups,
  notification occurrences and counts, user profiles, presence, active calls,
  and the paged room and thread timelines (`server/`, `room/`).
- The client runtime: recovery, realtime ownership, and session termination
  (`server/runtime`).
- Asset URL helpers (`util/`).

The frontend keeps these parts:

- Components, routes, URL selection of the active server, and Svelte context.
- Translated text, toasts, and sounds. Stores keep the original error object.
  The frontend makes the message with `errorMessage()`.
- Its one client (`$lib/client`), with device storage, the origin server,
  and one live server.
- The UI state of each server (`$lib/state/server/serverUi`): read views and
  notification attention, sidebar navigation with notification counts,
  optimistic unread and membership state, pending highlights, message search
  sessions, the admin room-layout editor, and the LiveKit voice call with its
  call overlays.
- Timeline scroll positions and jumped mode (`$lib/state/room`). The client
  keeps the paged window and the anchor event that a reset reloads around.
- The snapshot query cache on TanStack Query (`$lib/query`) and the
  view-shaped API modules for admin tools, first-run setup, Web Push, and the
  cross-tab session channel (`$lib/api`, `$lib/auth`).
- Page lifecycle tracking and device UI preferences, such as the presence
  choice.

### Server data and host state

The client keeps server data and the operations on it. A host keeps its own
state next to each server:

- Host state that derives from server data needs no cleanup: it follows the
  server. Examples are sidebar groups and badge counts.
- A host that copies server data, for example search results or cached
  reads, clears the copy at the server's boundary events: `onReset`,
  `onRoomAccessLost`, `onRoomAccessRestored`, `onUserDeleted`,
  `onAuthorityChanged`, `onPermissionsChanged`, `onSessionEnded`, and
  `onDispose`. `onUpdate` reports each applied event and resource. The server
  emits each event synchronously, inside the write that crossed the
  boundary, after it cleared its own copies.
- A failing listener of a privacy boundary fails the private-data cleanup:
  the server does not report its projection as current until a later reset
  succeeds. The server waits for the promises of `onPermissionsChanged`
  listeners before it reports the permission check as complete.
- `registry.watchStores` gives a host each server when the registry creates
  it, before realtime data arrives.

The client scrubs its own data at every boundary. It cannot make a host clear
the host's copies; a copy is the host's to clear.

We considered host extensions that the client creates through factories and
calls through optional hooks, as the first voice-call integration did. We
rejected them: the client would still know the host's state, and an optional
hook does not make an extension clear its copies either.

### Reactivity

The package has its own small signal library in `reactivity/`: `signal`,
`computed`, `effect`, `effectRoot`, `batch`, `untrack`, `subscribe`,
`ReactiveMap`, and `ReactiveSet`. A computed value is lazy. A computed value
without observers does not register with its sources, so it can be garbage
collected with its owner. Effects run synchronously when the outermost write
or `batch` ends. An effect error is logged; it does not reach the code that
wrote the signal. Multi-step store operations, such as registry changes, run
in one `batch`, so effects never observe half-applied state.

We examined alien-signals, Preact signals, `@vue/reactivity`, TanStack Store,
and the TC39 signal polyfill. None of them has a hook for reads outside the
graph together with per-key collections. The Svelte adapter needs this hook.
The library is small and is tested in the package.

Store state is a private signal with a getter and a setter of the same name.
So consumers read `store.status` and do not call a function.

`@chatto/client/svelte` connects the library to Svelte. An import of this
module installs a read hook. For each node that a Svelte reaction reads, the
hook uses one `createSubscriber`. Svelte subscribes to the node only while a
reaction tracks it. The frontend imports the adapter in `hooks.client.ts`, in
the Vitest browser setup, and in Storybook.

### Clients and connections

- A client is isolated. Its registry, stores, connections, realtime
  transports, and runtime belong to it alone. State that stays process-wide,
  such as the snapshot query cache and user stores, is keyed by server ID,
  and a server ID belongs to one client in a process.
- Client options select the storage (`memory`, the default, or `device`), the
  origin server with its cookie session, and the live servers (`all`, the
  default, or one `selected` server). A process can have one client with
  device storage and one for the origin server.
- `client.connect({ serverUrl, apiKey })` adds a server with a fixed token
  and returns it. `client.server(id)` returns a server that the client has.
  Both return the same `Server` type, so a bot and the frontend use the same
  object. A fixed token is never renewed or written to device storage. The
  server's rejection or a session termination ends the session. A browser
  page cannot connect its own origin this way, because those requests also
  carry the page's cookie session. A server that `connect` added is always
  live. The client runs its runtime while it has such servers, or after the
  application calls `start()`. `server.close()` removes a server from its
  client.
- `server.run(handler)` waits until the server accepted the key, also
  through retries while the server is unreachable, then handles the messages
  addressed to the viewer in order and gives each one a context with the
  operations to answer it.
  `server.consumeEvents` is the ordered loop for all events.
- `createApi({ serverUrl, apiKey })` makes stateless typed requests with
  Connect JSON and has the same request helpers. It rejects redirects and does
  not load the stores. Use it for short work, such as a webhook handler, and
  for work that can outlive a connection.

### Workspace consumption

The package exports each module as `@chatto/client/<path>`. It also exports
`.`, `./svelte`, and `./reactivity`. The `@chatto/source` export condition
points to the TypeScript source. The frontend's Vite and TypeScript settings
use this condition, so the frontend builds the client from source. Node hosts
use the compiled `dist/` output.

The package keeps the MIT license of the previous `@chatto/client` package.
Most of its code comes from `apps/frontend`, where it was Apache-2.0. This
decision moves that code across the license boundary: in the package, it is
MIT. `REUSE.toml` records the package boundary.

## Consequences

- The client is tested in the package with Vitest. Most tests use happy-dom
  because they cover browser behavior, such as the origin server and storage.
  A test makes sure that core modules do not import Svelte or frontend code.
- The frontend creates the UI state of each server when the registry creates
  the server, outside reactive reads, so that Svelte tracks the voice call's
  `$state`. The same watch connects the query cache to the server's boundary
  events.
- Frontend specs with hand-written server mocks mock `serverUi` so that the
  mock also carries the UI state; `createTestServerScope` does this itself.
- Bots receive the frontend's recovery, projection, and privacy fences.
- A module can no longer reach a global registry. Code that needs one
  receives it from its client, and the frontend imports its client from
  `$lib/client`. Its tests mock that module.
- ChattoBot closes its connection at the end of each Runling source
  generation. A new generation starts from a new realtime snapshot. Messages
  that arrive between generations are not replayed. The ADR-100 client
  replayed them from an in-memory checkpoint.
- Signals compare values by reference. Stores replace arrays and objects to
  publish a change; they do not mutate reactive state in place.
- In a Svelte app, a read of a reactive collection key outside a client
  computed or effect creates a per-key signal, because the adapter cannot see
  whether Svelte tracks the read. The signal is removed with its key.
- Each read of a client node in a tracking Svelte reaction creates a small
  Svelte render effect, also when the reaction reads the same node again. In a
  loop, read a store value once before the loop. The rules of ADR-010 still
  apply: put only renderable data into reactive collections.
- Client code must not use `window`, `document`, or `localStorage` without a
  guard. Node hosts do not have them.
- The package is not published to npm yet. Its module paths are not a stable
  public API.
