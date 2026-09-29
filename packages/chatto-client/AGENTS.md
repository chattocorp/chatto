# Instructions for Agents Working in `packages/chatto-client/`

`@chatto/client` is the Chatto client: ConnectRPC facades, sessions, the
realtime transport and projection, the server and room stores, the snapshot
query cache, and the client runtime. The bundled frontend, ChattoBot, and the
Runling examples use it. See
[ADR-110](../../docs/adr/ADR-110-move-client-state-into-chatto-client.md).

## Boundary

- Keep the package framework-neutral. Only `src/svelte/` may import Svelte.
  Do not use runes, `svelte/reactivity`, or `$lib`/`$app` imports anywhere
  else. `src/reactivity/boundary.test.ts` enforces this.
- Keep UI concerns in the application: translated text, toasts, sounds,
  navigation, routes, and media. Stores keep error objects and report
  outcomes; applications turn them into messages.
- Use an interface and an installable implementation for application
  capabilities, such as `VoiceCallController` and `setVoiceCallFactory`.
  Register application types through `Register` in `src/register.ts`.
- Guard every use of `window`, `document`, `navigator`, and `localStorage`.
  Node hosts, such as bots, do not have them or have only part of them.
- Do not add a dependency on Runling, Authling, or an application package.
- Pin `@tanstack/query-core` to the version that the frontend's
  `@tanstack/svelte-query` uses. The frontend binds Svelte Query to the
  package's `QueryClient`; `apps/frontend/src/lib/query/client.spec.ts`
  fails when the versions differ.
- `connectChatto` and applications each start a client runtime. A process
  runs one runtime at a time.

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
- The root entry is for headless hosts: `connectChatto` and `createChattoApi`.

## Tests

- Run `mise test-chatto-client` for type checks and tests.
- Tests use happy-dom by default, with in-memory `Storage` and Web Locks from
  `src/testing/setup.ts`. Add `// @vitest-environment node` for tests of Node
  hosts.
- Use `fakeServer` from `src/testing/fakeServer.ts` for API tests. It runs the
  real generated clients and interceptors against an in-memory Connect router.
- After a change that frontend code depends on, also run the frontend checks
  and tests; see [apps/frontend/AGENTS.md](../../apps/frontend/AGENTS.md).
