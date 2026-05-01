# Frontend Testing

## Choose the right layer

Pick the lowest layer that can give you a real signal. E2E tests are slow and brittle (`retries: 3` in `playwright.config.ts`); reach for them only when the behavior **genuinely** needs a real backend, NATS subscriptions, multi-user, or cross-route navigation.

| Layer | Use it when... |
| --- | --- |
| **Pure unit (`.spec.ts`)** in the `server` Vitest project | The code is a pure function or a class whose dependencies you can pass in directly (formatters, parsers, validators, fuzzy matchers, virtual-list builders, transforms). |
| **Browser/component (`*.svelte.spec.ts`)** in the `client` Vitest project | You need real DOM, `localStorage`, drag-and-drop events, real fonts/CSS, or you're rendering a Svelte component. |
| **E2E (`frontend/e2e/*.test.ts`)** | The behavior under test only exists when the real GraphQL gateway, NATS, and at least one real user session are in the loop. |

## Match the test to the change

The "Choose the right layer" table above tells you which layer to *write* a test in. This subsection is the inverse: given the kind of change you just made, which layer is the *minimum* needed to verify it actually works. A change that crosses a layer must be tested at (at least) that layer.

| Change | Minimum layer | Why pure unit isn't enough |
| --- | --- | --- |
| Pure function / formatter / data transform | Pure unit | (it is enough) |
| State store class API (mutator, derivation, selector) | Pure unit on the store | (it is enough — but see next row if a component depends on it) |
| Component reads new store property in template | Browser/component | The template binding is what you changed; verify it renders. |
| Component `$effect` that reads + writes store/context state | Browser/component (mount the component) | `effect_update_depth_exceeded` is a runtime guard that only fires from a mounted component. Pure store tests never trip it. |
| Adding/removing a context provider or consumer | Browser/component | Missing-context errors fire at mount, not at construction. |
| Subscription handler in a layout or store (event bus, GraphQL subscription) | Browser/component with a stubbed subscription, OR e2e | The handler only runs when the subscribing component is mounted and an event arrives. |
| Cross-instance behavior (two real backends, real WebSockets) | E2E | The browser project can't run two GraphQL gateways. |
| URL/router behavior (navigation, params) | E2E or a component test using `$app/navigation` mocks | SvelteKit routing requires a real or stubbed routing context. |

If your change spans rows, the highest-row layer is the floor — a refactor that touches a store **and** how a component effect uses it needs a mounted-component test, not just a store unit test.

A common trap: a refactor that "only" moves orchestration from a component into a store still changes the component (it now reads/writes through a different surface). That's a component-level change. Mount it.

## Where do specs live

Co-locate next to the source. The Vitest project split is purely by filename suffix (see `frontend/vite.config.ts`):

- `foo.ts` → `foo.spec.ts` runs in the **server** project (Node).
- `Foo.svelte` or `foo.svelte.ts` → `Foo.svelte.spec.ts` or `foo.svelte.spec.ts` runs in the **client** (browser) project.

`.test.ts` and `.spec.ts` are both accepted; existing files use both. Match the surrounding directory.

## Use the shared helpers

`frontend/src/lib/test-utils/` exists to keep boilerplate out of specs. Don't re-roll any of these per file:

```ts
import {
  q,                       // querySelector with HTMLElement cast
  testSnippet,             // Snippet from raw HTML for component children
  createMockGraphqlClient, // typed urql Client mock
  createMockConnection     // mock for useConnection() shape
} from '$lib/test-utils';
```

A typical component spec with a GraphQL mutation:

```ts
import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MyComponent from './MyComponent.svelte';
import { createMockConnection, createMockGraphqlClient, q } from '$lib/test-utils';

const mutationData = { thing: { id: 'x' } };

vi.mock('$lib/state/instance/connection.svelte', () => ({
  useConnection: () => () => createMockConnection({ mutationData })
}));

describe('MyComponent', () => {
  it('renders the form', async () => {
    const client = createMockGraphqlClient({ mutationData });
    const { container } = render(MyComponent, {
      props: { ... },
      context: new Map([['$$_urql', client]])
    });
    await expect.element(q(container, 'button[type="submit"]')).toBeInTheDocument();
  });
});
```

## Conventions

- **Always have explicit assertions.** `expect: { requireAssertions: true }` is on globally — empty tests fail.
- **Use `expect.element(...)` for DOM assertions.** It auto-retries; bare `expect(el)` does not. The `q()` helper exists because `expect.element()` needs `HTMLElement`, not `Element`.
- **Flush after state changes.** When a test calls a function that mutates Svelte `$state` and then queries the DOM, call `flushSync()` from `svelte` first. See `AutocompletePopup.svelte.spec.ts` for the pattern.
- **Singletons need exported classes.** `vi.resetModules()` does not re-instantiate ESM module-level singletons in browser mode. If you need to test constructor-time hydration (e.g. `localStorage` reads), export the class so the spec can `new` a fresh instance per test. See `recentReactions.svelte.ts` and its spec.
- **Mock at the boundary, not deeper.** Mock `'$lib/state/instance/connection.svelte'` (the surface the component imports) instead of mocking urql internals.
- **Don't test what you can derive.** No need to assert that a button has `cursor-pointer` if the parent class is enforced by Tailwind config — focus on observable behavior.

## When you're tempted to write an e2e

Ask:

1. *Could this be a component test if I mocked the GraphQL response?* Usually yes, especially for forms, autocompletes, modals, validation, and keyboard handling.
2. *Could the deterministic part be a unit test, with a small e2e for the integration glue?* This is the "split" pattern — most of the logic lives in a fast spec, and one e2e proves the wiring.
3. *Does the e2e you're considering exercise scroll position, virtua, real CSS, or `localStorage`?* The browser Vitest project gives you all of that.

If after that the answer is still "yes, this needs e2e," go ahead. But do the unit/component test first; the e2e is then a smaller, more targeted smoke test.

## Running tests

```sh
mise test-frontend                            # full suite (server + client)
mise x -- pnpm test:unit                      # watch mode in frontend/
mise x -- pnpm test:unit --run --project client src/path/to/Foo.svelte.spec.ts
```

The full suite should stay well under 10 seconds on a developer machine; if a single browser-mode spec balloons past ~1s, the test is probably doing too much — split it.
