---
paths: ["frontend/**"]
---

# Frontend Conventions

Conventions, UI patterns, and Svelte 5 idioms for working in `frontend/`. Companion to [`frontend.md`](frontend.md), which covers the deeper architectural patterns (state stores, multi-server architecture, lifecycle timing).

## Browser Support

Target modern evergreen browsers only. No support for Internet Explorer or legacy browsers.

- Chrome/Edge 90+ (2021)
- Firefox 90+ (2021)
- Safari 14+ (2020)

Use modern Web APIs (`navigator.clipboard`, `fetch`, `async/await`, etc.) without fallbacks.

## Formatting and Tooling

- Svelte files use tabs for indentation. Match the existing indentation style of the file being edited.
- Use `pnpm` for package management and running scripts.

## Type Generation

Frontend TypeScript types for permission constants and other shared Go types are generated via `tygo`. When Go types are renamed or added:

1. Run `mise codegen-types` to regenerate `frontend/src/lib/types/core.ts`
2. Update all frontend imports that reference the changed types
3. The TypeScript compiler will catch mismatches, but e2e tests may fail with confusing errors (like "Access Denied") if types are stale.

**Common symptom**: E2e tests fail with permission errors even though backend tests pass. This often means the frontend is checking against old permission constant values.

## GraphQL Client

- Use `graphqlClient.client` from `$lib/state/graphqlClient.svelte.ts` for mutations/queries.
- Do **NOT** use `getContextClient()` from `@urql/svelte` — the app uses a singleton, not context.
- **No GraphQL cache**: The client has no cache exchange — queries always fetch fresh data.
- **Event-driven updates**: Instead of relying on cache invalidation, components subscribe to real-time events (via event buses) and update their local state. This fits the WebSocket-based architecture.
- **Type imports**: Generated GraphQL types are in `$lib/gql/graphql`, not the `$lib/gql` index:
  ```typescript
  import { graphql } from "$lib/gql"; // For the graphql tag function
  import type { SomeType } from "$lib/gql/graphql"; // For generated types
  ```

## Event Buses

Three buses, one for each event family:

- **Instance events** (`instanceEventBus.svelte.ts`) — space/user-level changes. Subscribe via `onInstanceEvent()`.
- **JetStream events** (`spaceEventBus.svelte.ts`) — messages, room events. Subscribe via `onSpaceEvent()`.
- **Live events** (`spaceLiveEventBus.svelte.ts`) — reactions, typing. Subscribe via `onSpaceLiveEvent()`.

**Adding a new instance event handler** (e.g., for `UserProfileUpdatedEvent`):

1. Add the event fields to the subscription in `instanceEventBus.svelte.ts`.
2. Create a typed handler hook (e.g., `onUserProfileUpdate`) that filters for that event type.
3. Subscribe in components using `$effect(() => onUserProfileUpdate(...))` — the hook returns cleanup.

```typescript
// In instanceEventBus.svelte.ts
export function onUserProfileUpdate(handler: UserProfileHandler): () => void {
  const bus = getContext<InstanceEventBus | undefined>(INSTANCE_EVENT_BUS_KEY);
  if (!bus) return () => {}; // Graceful fallback if bus not initialized

  const wrapper: EventHandler = (event) => {
    if (!event.event) return; // Skip unknown event types
    if (event.event.__typename === "UserProfileUpdatedEvent") {
      handler({ userId: event.event.userId, avatarUrl: event.event.avatarUrl });
    }
  };
  bus.handlers.add(wrapper);
  return () => bus.handlers.delete(wrapper);
}

// In component
$effect(() =>
  onUserProfileUpdate((update) => {
    if (update.userId === user.id) liveAvatarUrl = update.avatarUrl;
  }),
);
```

**Whitelist cacheable events in SpaceEventProvider**: when routing subscription events to the message cache, use an explicit whitelist of displayable event types — not a blacklist.

**Event handler null checks**: always check for `event.event` being null before accessing properties.

## Real-time Update Patterns

- **Refetch-on-event**: when receiving live events, refetch from server for consistency.
- **Inline cache updates**: when the incoming event contains all data needed, update the cache directly.

## Modals

- Modals use SvelteKit shallow routing with `pushState('', { modal: { type: '...', ...context } })`.
- `App.PageState` in `app.d.ts` defines the type-safe modal state schema.
- Close modals with `history.back()`.
- Use modals for quick actions (create space, create room). Use full pages for settings and discovery/browsing.

## Context Menus & Popovers

The architecture lives in [`frontend.md`'s "Floating UI" section](frontend.md) — always go through `FloatingPopover` or wrappers like `ContextMenu`/`HelpTooltip`. Operational details:

**Two positioning modes** on `ContextMenu`:
- **Point** (`position: { x, y }`) — for right-click context menus.
- **Anchor** (`anchor: { top, bottom, left }`) — for popovers attached to a trigger element.

**Interactive elements inside context menu triggers** need `oncontextmenu` with `preventDefault` + `stopPropagation` and `ontouchstart` with `stopPropagation`.

**Virtualized lists**: `virtua` applies `contain: layout` to list items. That's why we render overlays through the Popover API (top layer) — direct positioning escapes the containment.

## Form Components

- Use components from `$lib/ui/form` (`TextInput`, `TextArea`, `Select`, `Checkbox`, `Button`, `FormError`).
- Pass `error` prop to inputs for field-level validation display.

## Form Validation

Use Zod for validation — import `z` and `validate` from `$lib/ui/form`. Define schemas at component level, use `$derived` for reactive error display:

```typescript
import { z, validate } from "$lib/ui/form";

const emailSchema = z.email({ error: "Please enter a valid email" });
const emailError = $derived(email ? validate(emailSchema, email) : undefined);
```

## Page Layout

Pages rendered in the main content area must wrap all content in a single flex column container:

```svelte
<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Page Title" subtitle="Description" />
  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <!-- Page content -->
  </div>
</div>
```

## UI Guidelines

- Prefer plain HTML + Tailwind utilities over wrapper components.
- Use native elements (e.g., `<dialog>` for modals).
- Always add `cursor-pointer` to clickable elements.
- **Mobile tap targets**: use responsive sizing for touch devices.

## Permission-Based UI Gating

Query `viewerCan*` fields on the Space type and conditionally render or disable UI. For checks that depend on locally available data (like role hierarchy), compute them in the frontend rather than adding backend fields.

## Shared Component Libraries

- `$lib/components/menus/` — context menu content components
- `$lib/components/rbac/` — role management components
- `$lib/components/admin/` — admin panel components
- `$lib/ui/form/` — form components
- `$lib/ui/` — layout components

## CSS Utilities

A library of utility classes lives in `app.css`. Use the `skeleton` utility class on `<img>` elements for loading state.

### CSS Truncation in Flex Layouts

For `truncate` to work in flexbox, **every flex ancestor** needs `min-w-0`. (See also `general.md` for the related "image attachments overflowing on mobile" gotcha.)

## PWA Setup

- Manifest at `static/manifest.webmanifest`, linked from `app.html`.
- Icons in `static/icons/`.
- Use `sharp` dev dependency + `scripts/generate-icons.mjs` to regenerate icons from SVG.

---

# Svelte 5 Idioms

- Always use Svelte 5 idioms, not Svelte 4 patterns (e.g., `<svelte:document>` over legacy event handlers, `$derived` over reactive statements).
- Use runes (`$state`, `$derived`, `$effect`) — no legacy reactivity.
- Experimental async Svelte is enabled — use where appropriate ([docs](https://svelte.dev/docs/svelte/await-expressions)).
- Don't put too much code into a single component — break into smaller components or modules as needed.
- Don't place reusable components or supporting modules in route directories. Shared components belong in `$lib/components/`, and shared state modules in `$lib/state/`.
- Prefer Tailwind classes over `<style>` blocks for styling components.
- Keep Tailwind classes in the `class` attribute directly using array syntax: `class={['base', condition ? 'a' : 'b']}`.
- Use Svelte 5's `resolve()` from `$app/paths` for typechecked URL paths. (See `frontend.md`'s Navigation section.)
- **`resolve()` exceptions**: some URLs legitimately cannot use `resolve()`. Use `eslint-disable` comments with clear reasons.

Document components with `@component` in an HTML comment before the `<script>` tag:

```svelte
<!--
@component

Brief description of what the component does.

**Props:**
- `name` - Description of prop
-->
<script lang="ts">
  ...
</script>
```

## Svelte 5 Reactivity Pitfalls

The big architectural pitfalls (`$effect` overuse, context getters, reactive props as getters) live in `frontend.md`. These are the smaller foot-guns:

### Effect Dependencies Without Unused Variables

Use `void` to avoid ESLint unused-variable warnings when you need a dependency for its reactivity only:

```typescript
$effect(() => {
  void roomId; // Creates dependency, no lint warning
  shouldScrollToBottom = true;
});
```

### Writable `$derived` (Svelte 5.25+)

`$derived` values can be directly overwritten. Useful for "prop with real-time override" patterns. **Caution**: writable `$derived` can cause subtle reversion bugs if the source value changes unexpectedly.

### Reactive Cache Mutations and Derived Chains

Before mutating a reactive cache, verify the item will survive all downstream filters. If it won't, don't add it.

### `Intl.DateTimeFormat` Is Expensive

Cache `Intl.DateTimeFormat` instances when calling repeatedly with the same options.

### Method Calls in Templates

Calling store methods directly in templates may not establish reactive dependencies. Wrap in `$derived`.

### `{#each}` and External State

`{#each}` only re-renders when the iterated array reference changes. For external state dependencies, use `{#key}`.

### Reset State on Entry, Not Cleanup

Reset state in an effect that depends on the new prop value, not in a cleanup function.

### Context and Async Callbacks

`getContext()` can only be called during component initialization. Return update functions with closures during initialization.

### Transitions in Static Conditional Blocks

Use `|global` when a transitioned element is inside a block whose condition is effectively constant.

## State Management Classes

Prefer classes with `$state` fields for stores and state containers. Context-scoped stores still need a factory to call `setContext()`, but the factory should instantiate a class.

### `SvelteMap` / `SvelteSet` Usage

`SvelteMap` and `SvelteSet` from `svelte/reactivity` are reactive for their method calls. They do **NOT** need `$state` wrapping. Always use methods instead of reassignment.

## Attachments (`{@attach}`) vs Actions (`use:`)

Svelte 5 replaces `use:action` with `{@attach}` for attaching behavior to DOM elements.

## SvelteKit

- Static SPA only — no server-side rendering (`ssr = false` at root layout).
- Client-side load functions (`+page.ts`, `+layout.ts`) are fine and encouraged.
- Never use server load functions (`+page.server.ts`, `+layout.server.ts`).
- Add dependencies as devDependencies (bundled at build time).

### Layouts

- Use `+layout.svelte` for shared UI.
- Use `@render children?.()` for slot content.
- For active link detection, read `page.url.pathname` from `$app/state`.

### `$app/state` vs `$app/stores`

Prefer `page` from `$app/state` over `$page` from `$app/stores`.

### Refreshing Auth State After Login

After login/registration, use `invalidateAll()` to force SvelteKit to re-run load functions with fresh session data.

### Load Functions for Param Extraction

Use `+page.ts`/`+layout.ts` load functions to extract route params and query strings.

---

# Testing

When UI elements change, update e2e test selectors accordingly. Check `frontend/e2e/fixtures/` when modifying shared UI patterns. See [`testing-frontend.md`](testing-frontend.md) for the full testing guide.
