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
| Cross-server behavior (two real backends, real WebSockets) | E2E | The browser project can't run two GraphQL gateways. |
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

vi.mock('$lib/state/server/connection.svelte', () => ({
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
- **Mock at the boundary, not deeper.** Mock `'$lib/state/server/connection.svelte'` (the surface the component imports) instead of mocking urql internals.
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

## Running e2e tests locally

**E2E does NOT need Docker / `mise dev` / OrbStack.** Each Playwright test spawns its own `chatto` binary with embedded NATS on a random per-worker port (see `e2e/fixtures/server.ts`). The Playwright `globalSetup` calls `mise build-e2e-server` on every run; mise's source/output tracking turns that into a no-op when nothing has changed and a real rebuild when backend code has, so you don't need a manual rebuild step when iterating.

```sh
mise test-e2e                                                    # full e2e suite
mise x -- pnpm exec playwright test e2e/dm.test.ts               # one file
mise x -- pnpm exec playwright test e2e/dm.test.ts -g "post a"   # one test
mise x -- pnpm exec playwright test e2e/dm.test.ts --retries=0   # no retries (faster signal while iterating)
```

**Always start with the e2e tests most likely to exercise what you changed; only run the full suite once those pass.** The full suite takes minutes and retries failures three times — a broken fixture or core regression compounds that delay. Pick the related spec(s) first, get them green, *then* run `mise test-e2e`. The right shape is:

1. Identify the e2e files that exercise the area you touched (`e2e/dm.test.ts` for DM changes, `e2e/room-membership*.test.ts` for membership, etc.).
2. Run those with `--retries=0` to get a fast signal.
3. Fix anything that breaks. Re-run the targeted spec(s) until clean.
4. Only then `mise test-e2e` for the full sweep — usually catches unrelated breakage you wouldn't have suspected.

If a test hangs or times out without a clear assertion failure, look at `frontend/test-results/<test-name>/error-context.md` — it includes the page snapshot and the failing line, which usually pinpoints the problem faster than the console output. When the snapshot belongs to the wrong `Page` (e.g. you have `page` + `regularPage`), the captured page is the test's primary one; add a temporary `console.log` against the other page or capture a screenshot to see its state.

## E2E gotchas worth remembering

- **`page.waitForLoadState('networkidle')` will hang on a flapping WebSocket.** When the backend rejects a graphql-ws subscription (e.g. permission denied), the client retries forever and `networkidle` never fires. Don't gate assertions on `networkidle` after a permission change; assert directly on the DOM. And on the client side: gate subscription *creation* on the relevant permission so the loop never starts (see `ServerEventProvider.svelte`'s `dm.view` guard).

- **Test message bodies must be unique.** `getByText('hi')` collides with empty-room boilerplate ("You've reached the very beginning of this conversation.") and trips strict-mode. Use `${Date.now()}` or `crypto.randomUUID().slice(0, 8)` in any string the test then asserts on.

- **`locator.filter({ has: ... })` matches every ancestor.** `page.locator('div.relative').filter({ has: page.getByTestId('space-icon') })` matches both the SpaceIcon's wrapper *and* the Server Gutter layout div *and* the chat chrome div, all of which contain a space-icon descendant. Scope the parent locator to a unique class first (e.g. `page.locator('.server-gutter div.relative')`).

## E2E Test Isolation

Each e2e test runs against its own isolated Chatto instance. This means:

- Tests don't share state (users, spaces, rooms, permissions).
- No cleanup is required between tests.
- Tests can safely create users/spaces without worrying about collisions.
- Permission changes in one test don't affect others.

The test infrastructure spins up a fresh Chatto server for each test file, so you don't need to restore state after modifying permissions or instance-level settings.

## Page Object Models

Use Page Object Models (`frontend/e2e/pages/`) to encapsulate UI interactions:

- **`ChatPage`** — sidebar navigation, space creation, room entry
- **`RoomPage`** — message input/sending, attachments, thread pane
- **`MessageComponent`** — per-message actions (react, delete, edit, threads)
- **`ExplorePage`** — space discovery and joining

POMs are injected via Playwright fixtures in `setup.ts`. For multi-user tests with a second browser context, instantiate POMs directly: `const chatPage2 = new ChatPage(page2)`.

## Multi-User Real-Time E2E Tests

Real-time features require testing that events are visible to **other** users, not just the actor. A common gap: testing that User A's action succeeds, but not that User B sees the resulting event.

```typescript
test("user sees leave event when another user leaves", async ({
  page,
  browser,
  serverURL,
}) => {
  const user1 = await createAndLoginTestUser(page);

  const context2 = await browser!.newContext({ baseURL: serverURL });
  const page2 = await context2.newPage();

  try {
    const user2 = await createAndLoginTestUser(page2);

    await expect(page.getByText(`${user2.displayName} joined`)).toBeVisible({
      timeout: 5000,
    });

    await page2.getByTitle("Leave room").click();

    await expect(page.getByText(`${user2.displayName} left`)).toBeVisible({
      timeout: 5000,
    });
  } finally {
    await context2.close();
  }
});
```

**Test both directions**: if User A can trigger an event, test that User B receives it.

### `verifyRealtimeSync` Limitations

The `verifyRealtimeSync` helper from `fixtures/realtimeSync.ts` only works when the receiver is at the bottom of the chat (auto-scroll enabled). If the receiver is scrolled up, the sync message won't be visible and the assertion will fail.

- **Use `verifyRealtimeSync` when** both users are at the bottom of the chat and you need to prove WebSocket subscriptions are connected before testing.
- **Use `waitForRoomReady` instead when** the receiver might be scrolled up, or you just need to ensure the room UI is loaded. Combine with `TIMEOUTS.REALTIME_EVENT` for assertions.

### Multi-Tab Sync Tests Need All UI Levels

When testing multi-tab/multi-device sync for indicators (like unread dots), verify ALL levels of UI sync, not just one:

```typescript
// COMPLETE — tests both space-level AND room-level
await expect(page3.locator('[data-testid="space-unread-dot"]')).not.toBeVisible();
await expect(page3.locator('[data-testid="room-unread-dot"]')).not.toBeVisible();
```

### Server State Synchronization

For multi-user tests that depend on server state changes (like unread indicators), **don't use arbitrary timeouts**. Poll the server via GraphQL until the expected state is reached:

```typescript
import {
  waitForSpaceUnread,
  waitForRoomUnread,
  waitForRoomRead,
} from "./fixtures/graphqlHelpers";

await waitForSpaceUnread(page, spaceId, true);
await waitForRoomUnread(page, spaceId, roomId, true);
await waitForRoomRead(page, spaceId, roomId);
```

## Permission Tests in E2E

### Default Permission Changes Cascade to E2E Tests

When modifying default role permissions (e.g., removing `room.create` from `everyone`), e2e tests break if regular members perform those actions. The tests time out rather than showing permission errors because UI elements are hidden.

**Fix pattern**: have the space owner perform privileged actions; have regular members use alternate flows.

```typescript
// Before: user B (regular member) creates room — breaks if room.create removed
await chatPage2.createRoom();

// After: user A (owner) creates room, user B joins via Browse Rooms
const roomName = await chatPage.createRoom();
await page2.getByRole("link", { name: "Browse Rooms" }).click();
await page2
  .locator("li", { hasText: `# ${roomName}` })
  .getByRole("button", { name: "Join" })
  .click();
```

### Negative Permission Tests

Test both directions: with the permission, the action works; without it, access is denied. UI route guards and component visibility both need coverage. (See `testing-backend.md` for the equivalent rule at the resolver layer.)

## E2E Scrolling with `virtua`

The event list uses `virtua` for DOM virtualization. Programmatic `scrollTop` assignment doesn't work reliably with virtua's scroll correction. Tests must use **native mouse wheel events**:

```typescript
async function scrollContainerToTop(page: Page, container: Locator) {
  const box = await container.boundingBox();
  if (!box) throw new Error('Container not visible');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let i = 0; i < 15; i++) {
    await page.mouse.wheel(0, -800);
    await page.waitForTimeout(50);
  }
}
```

### Scroll Stabilization After Bulk Posting

After posting messages via API, the UI may still be auto-scrolling. Wait for scroll position to stabilize before testing scroll behavior:

```typescript
await postMessagesViaAPI(page, spaceId, roomId, messages);
await expect(page.getByText('Message 20')).toBeVisible({ timeout: 5000 });

await expect(async () => {
  const info = await messagesContainer.evaluate((el) => ({
    scrollHeight: el.scrollHeight,
    scrollTop: el.scrollTop,
    clientHeight: el.clientHeight
  }));
  const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
  expect(distanceFromBottom).toBeLessThan(50);
}).toPass({ timeout: 5000, intervals: [100, 250, 500, 1000] });

await scrollContainerToTop(page, messagesContainer);
```

### Robust Scroll Position Assertions

Avoid exact scroll position comparisons. Test the actual behavior:

| Flaky                                                       | Robust                                            |
|-------------------------------------------------------------|---------------------------------------------------|
| `expect(scrollTop).toBe(0)`                                 | `expect(scrollTop).toBeLessThan(5)`               |
| `expect(scrollTopAfter - scrollTopBefore).toBeLessThan(50)` | `expect(distanceFromBottom).toBeGreaterThan(100)` |

## API-Based Message Posting for E2E Setup

When tests need many messages for setup (e.g., making a container scrollable), use GraphQL API calls instead of UI-based posting. This is ~10x faster and more reliable:

```typescript
async function postMessagesViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  messages: string[]
): Promise<void> {
  for (const body of messages) {
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
        variables: { input: { spaceId, roomId, body } }
      }
    });
  }
}
```

**Use UI-based posting only when testing the posting behavior itself** (e.g., "user posts while scrolled up" tests the scroll-to-bottom behavior triggered by sending).

## Selectors

### Specificity

Avoid `getByText()` for assertions — it often matches multiple elements. Prefer specific locators:

| Instead of                          | Use                                                       |
| ----------------------------------- | --------------------------------------------------------- |
| `getByText('Browse Spaces')`        | `getByRole('heading', { name: 'Browse Spaces' })`         |
| `getByText('Access Denied')`        | `getByText('Access Denied', { exact: true })`             |
| `getByText(displayName)` in sidebar | `locator('nav').getByRole('link', { name: displayName })` |

### Resilience

Avoid selectors that couple to specific HTML element types. Target semantic content (headings, alt text, roles) rather than structural elements.

### Form Selectors with `data-testid`

Use `data-testid` attributes for form elements. `TextInput` and `TextArea` components accept a `testid` prop. Naming convention: `{scope}-{component}-{element}`.

## UI Transition Timing

When switching between views, wait for old content to disappear first, then assert new content.

## Dialog Interception After `createRoom()`

After `chatPage.createRoom()`, the room creation modal may still intercept pointer events. Use `page.goto()` for navigation instead of clicking sidebar links.

## Avoid Default Room Names

Don't use names like `'general'` or `'announcements'` that conflict with system-created rooms.

## JavaScript Error Monitoring

Add tests that capture console errors and page errors for real-time event handling code paths. Errors that don't fail an assertion still indicate a bug.

## Playwright Fixture Naming

Playwright doesn't support underscore-prefixed fixture parameters (like `_page`). Solutions:
- Remove unused fixtures from destructuring entirely.
- Use destructuring rename: `{ chatPage: _chatPage }`.
- Use `eslint-disable-next-line no-empty-pattern` for empty patterns `{}`.
