# Instructions for Agents Working in `packages/chatto-client/`

`@chatto/client` is the Chatto client: isolated client instances with their
servers, realtime transports, and runtime; server data and its boundary
events; ConnectRPC facades and sessions; and the request helpers and message
loop for bots. The bundled frontend, ChattoBot, and the Runling
examples use it. See
[ADR-111](../../docs/adr/ADR-111-move-client-state-into-chatto-client.md).

## Boundary

- Keep the package framework-neutral. Only `src/svelte/` may import Svelte.
  Do not use runes, `svelte/reactivity`, or `$lib`/`$app` imports anywhere
  else. `src/reactivity/boundary.test.ts` enforces this.
- Keep UI concerns in the application: translated text, toasts, sounds,
  navigation, routes, and media. Stores keep error objects and report
  outcomes; applications turn them into messages.
- Keep client state in the client instance (`src/client.ts`): its registry,
  connection manager, realtime manager, and runtime. Do not add module-level
  state that one client could change for another. Process-wide state, such
  as the user stores, must be keyed by server ID; server IDs
  are unique in a process (`src/server/serverIds.ts`).
- Keep the package integration-shaped: server data, requests, and the
  operations that integrations need. State that exists only for one host's
  UI, such as search sessions, highlights, editors, or call media, belongs to
  that host. Do not call into host state from a store. Hosts derive from store
  state, and hosts that copy server data subscribe to the store's boundary
  events (`src/server/storeEvents.ts`). When a store crosses a new privacy or
  authorization boundary, emit the matching event.
- Guard every use of `window`, `document`, `navigator`, and `localStorage`.
  Node hosts, such as bots, do not have them or have only part of them.
- Do not add a dependency on Runling, Authling, or an application package.
- Request helpers take an options object with `signal` as their last
  argument. `MessagingRequests` holds them for connections and `Api`.

## Reactivity

- Use `src/reactivity/`: `signal`, `computed`, `effect`, `effectRoot`,
  `batch`, `untrack`, `subscribe`, `ReactiveMap`, and `ReactiveSet`.
- Expose store state as a private signal with a getter (and a setter when
  consumers write it) of the same name:

  ```ts
  readonly #statusSignal = signal<ConnectionStatus>('connecting');
  get status(): ConnectionStatus {
    return this.#statusSignal.get();
  }
  ```

- Use `computed` for derived state. Use `ReactiveMap` and `ReactiveSet` only
  for state that UI or computed values read. Use plain `Map` and `Set` for
  local scratch collections and listener sets.
- Signals compare values by reference. Replace an object or array to publish a
  change, or call `notify()` after an in-place mutation. ADR-010 applies: keep
  high-frequency data out of collections that render.
- Effects run synchronously when the outermost write or `batch` ends. Wrap a
  method that writes several signals in `batch`, so effects never observe a
  half-applied change. An effect error is logged and does not reach the writer.

## Modules And Exports

- Relative imports use `.js` extensions (NodeNext).
- Every module is importable as `@chatto/client/<path>`. The `@chatto/source`
  export condition resolves to TypeScript source for workspace consumers; Node
  hosts use `dist/`. Run `mise build-chatto-client` before Node consumers.
- The root entry has the public API: `createClient`, `Server`,
  `createApi`, the request helpers, and their types. `src/types.ts` and
  `src/types/admin.ts` re-export `@chatto/api-types`; regenerate them with
  `node scripts/generate-types.mjs`.

## Tests

- Run `mise test-chatto-client` for type checks, lint, and tests.
- The tests measure coverage. `vitest.config.ts` sets a floor just under the
  measured values, and the test run fails below it. Add tests for new code.
  Raise the floor when coverage increases. Do not lower it to make a change
  pass.
- Tests use happy-dom by default, with in-memory `Storage` and Web Locks from
  `src/testing/setup.ts`. Add `// @vitest-environment node` for tests of Node
  hosts.
- Use `fakeServer` from `src/testing/fakeServer.ts` for API tests. It runs the
  real generated clients and interceptors against an in-memory Connect router.
- Create the clients that a test needs. `createAppClient` from
  `src/testing/appClient.ts` creates a client configured like the frontend.
  Close clients after the test.
- After a change that frontend code depends on, also run the frontend checks
  and tests; see [apps/frontend/AGENTS.md](../../apps/frontend/AGENTS.md).
