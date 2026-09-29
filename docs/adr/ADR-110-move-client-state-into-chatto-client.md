# ADR-110: Move the Client State Layer into `@chatto/client`

**Date:** 2026-09-28

## Status

Accepted. Partially supersedes [ADR-100](ADR-100-shared-chatto-integration-client.md).
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
UI framework.

## Decision

`@chatto/client` (`packages/chatto-client/`) contains the Chatto client. The
package has no dependency on the frontend. Only the `@chatto/client/svelte`
adapter uses Svelte, as an optional peer dependency. The old integration
helpers are removed.

### Package contents

The package owns this client behavior:

- The ConnectRPC facades, the transport interceptors, and the privacy fences
  (`api/`).
- Sessions, bearer renewal, and origin cookie sessions (`auth/`, `server/registry`).
- Server connections and the realtime transport (`server/serverConnection`,
  `server/realtimeTransport`, `realtime/eventBus`).
- The realtime projection and the server and room stores (`server/`, `room/`).
- The snapshot query cache on `@tanstack/query-core` (`query/`).
- The client runtime: recovery, realtime ownership, and session termination
  (`server/runtime`).
- Page lifecycle tracking and asset URL helpers (`util/`).

The frontend keeps these parts:

- Components, routes, URL selection of the active server, and Svelte context.
- Translated text, toasts, and sounds. Stores keep the original error object.
  The frontend makes the message with `errorMessage()`.
- The LiveKit and audio implementation of voice calls. The package defines
  `VoiceCallController`. The frontend installs its implementation with
  `setVoiceCallFactory` and registers its type through the `Register`
  interface in `@chatto/client/register`.
- Svelte bindings for TanStack Query and device UI preferences.

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

### Headless hosts

- `connectChatto({ serverUrl, apiKey })` registers one server with a fixed
  token. It uses the same registry, stores, runtime, and realtime transport as
  the frontend. A process has one open connection at a time. The connection
  starts its own client runtime, and a process runs one runtime, so an
  application that runs a runtime cannot also use `connectChatto`. A fixed token is
  never renewed or written to device storage. The server's rejection or a
  session termination ends the session.
- `createChattoApi({ serverUrl, apiKey })` in `@chatto/client/apiClient` makes
  stateless typed requests with Connect JSON. It rejects redirects and does not
  load the stores. Use it for short work, such as a webhook handler.
- `@chatto/bot-client` builds bot conventions on these entry points:
  `createBotApi` for requests and `createBotClient` for an ordered realtime
  event loop. ChattoBot and the Runling webhook example use them.

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
- Bots receive the frontend's recovery, projection, and privacy fences.
- The registry, connection manager, and event bus manager stay module
  singletons. A process has one client. A later change can move them into a
  client instance.
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
