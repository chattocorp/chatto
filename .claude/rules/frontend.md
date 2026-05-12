---
paths: ["frontend/**"]
---

# Frontend Development

## Architecture Direction: State Stores + Thin Components

The frontend is moving toward this shape, and new code should follow it:

1. **Centralized state lives in store classes**, not in component-local `$state` or in ad-hoc query/subscription wiring inside components. A "store" here means a Svelte 5 class (or factory) with reactive `$state` / `$derived` properties, exposed via the per-server `ServerStateStore` bundle in `serverRegistry`, via singletons, or via context. **State that's logically "per server" (rooms, notifications, room directory, current user, voice call, …) belongs in `ServerStateStore`** — see "Per-Server State" below. Examples to mirror: `frontend/src/lib/state/space/rooms.svelte.ts`, `frontend/src/lib/state/server/notifications.svelte.ts`, `frontend/src/lib/state/server/store.svelte.ts`.
2. **Stores own their data lifecycle.** They issue the GraphQL query, ingest subscription events, and expose mutator methods (e.g. `markRead(roomId)`, `setMention(roomId)`). Components do not call `client.query(...)` or wire `client.subscription(...)` directly except to forward the event into a store.
3. **Components render from store state.** They read store properties in templates, call store methods on user actions, and add their own `$state` only for local UI concerns (open/closed, hover, draft input). If a component is doing data orchestration, that orchestration belongs in a store.
4. **Colocate GraphQL fragments with the components that read them**, and let stores consume those fragments via `useFragment`. The shape "what fields does this card need?" is a property of the card; the store doesn't need to know. Existing examples: `RoomEventView` fragment in `RoomEvent.svelte`, `UserAvatarUser` in `UserAvatar.svelte`. Stores compose fragments in their queries via `${FooFragmentDoc}` interpolation.

When refactoring, prefer:

- Pulling data orchestration out of `*.svelte` files into a `*.svelte.ts` store, even if there's only one consumer today. This is the canonical shape; one consumer is the start of the pattern, not an exception.
- Replacing `client.query` / `client.subscription` calls inside components with a store call.
- Extracting `$state` arrays that are mutated by subscription handlers into a store with explicit mutator methods (so the mutation surface is named and testable).

When NOT to add a store:

- Genuinely component-local UI state (modal open, focus, hover, drag position).
- Pure render helpers that don't hold state.

This direction is the lens I should apply when proposing simplifications: a refactor that moves us *toward* this shape is preferable to one that just moves code around within the old shape.

## Svelte 5 Lifecycle Timing

`$effect` runs AFTER the initial render pass completes. Component script initialization (top-level `<script>` code) runs synchronously DURING render. This means:

- Parent script init → Parent template renders → **Child script init** → Child template renders → `onMount` (children first) → `$effect`

**Gotcha:** If a parent's `$effect` creates a resource (e.g., an event bus), child components cannot access it during their script initialization — it doesn't exist yet.

**Fix:** Create resources synchronously in the parent's script section for the initial render. Use `$effect` only for reactive changes after mount (additions/removals). See `+layout.svelte` bus initialization pattern:

```svelte
// Synchronous: available to children during render
for (const server of serverRegistry.instances) {
  eventBusManager.startBus(server.id, client);
}

// Reactive: handles additions/removals after initial render
$effect(() => {
  // startBus is idempotent — no-op if already started
  for (const server of serverRegistry.instances) {
    if (!eventBusManager.getBus(server.id)) {
      eventBusManager.startBus(server.id, client);
    }
  }
  return () => { /* cleanup */ };
});
```

## Reserve `$effect` for actual side effects

`$effect` exists to synchronise the reactive graph with the outside world: DOM manipulation, subscriptions, timers, network calls, logging. **It is not a general-purpose "do this whenever something changes" hook.** Reach for it only when the work it performs has a real side effect.

For most other reasons to "react to a change," there's a more specific tool:

| You want to… | Use |
| --- | --- |
| Compute a value from other reactive state | `$derived` / `$derived.by` |
| Seed a `$state` from existing data once | Inline expression at `$state(...)` init |
| Forward an event to a store | An `on*` handler (or `useEvent`) |
| Talk to the DOM after a render | `$effect` — this is the canonical case |
| Subscribe to an external source | `$effect` with a cleanup return |
| Cache an expensive computation | `$derived` (memoised automatically) |
| Trigger a network request on mount/prop change | `$effect` (it's a side effect) |

**Anti-patterns (avoid):**

```ts
// ❌ Using $effect to initialise state from another reactive value
let displayName = $state('');
$effect(() => {
  displayName = user.displayName;  // not a side effect — just init
});

// ✅ Initialise directly. If `user` is a stable reference, just read it.
let displayName = $state(user.displayName ?? '');
```

```ts
// ❌ Using $effect to derive a value
let total = $state(0);
$effect(() => {
  total = items.reduce((sum, i) => sum + i.price, 0);
});

// ✅ This is exactly what $derived is for
const total = $derived(items.reduce((sum, i) => sum + i.price, 0));
```

```ts
// ❌ Using $effect to fan an event out to a handler
$effect(() => {
  if (events.length > prevCount) onNewEvent(events.at(-1));
  prevCount = events.length;
});

// ✅ The event source's on* / use* hook handles delivery for you
useEvent((event) => onNewEvent(event));
```

If you find yourself adding `$effect` because the warning "this reference only captures the initial value" fires on `$state(otherReactiveThing)`, the answer is almost never another `$effect` — re-shape the source so the read is non-reactive (capture a class instance into a plain `const`, then read its `$state` fields for init; see `routes/chat/[serverId]/settings/+page.svelte` for an example), or genuinely accept that you want a derived rather than a state.

## Context Getters Must Be Wrapped in `$derived`

When reading a context value that depends on async data (e.g., `getRoomPermissions()`, `getRoomMembers()`), **always wrap the call in `$derived`**. A plain `const` snapshots the value at script init time — if the underlying data hasn't loaded yet, you get the default/empty value permanently.

```ts
// BAD: snapshots DEFAULT_PERMISSIONS during init, never updates
const roomPermissions = getRoomPermissions();

// GOOD: re-evaluates reactively when the underlying data loads
const roomPermissions = $derived(getRoomPermissions());
```

This applies to any `getXxx()` context getter backed by a `$derived` or `$state` in a parent component. The getter chain only propagates reactivity when read inside a reactive context (`$derived`, `$effect`, or template expression).

## Pass Reactive Props as Getters to Non-Reactive Init Functions

When a `use*` hook or init function needs a prop value but runs during script init (non-reactive), **pass the prop as a getter** so the read happens inside `$effect` rather than at the call site. Never suppress `state_referenced_locally` warnings — they indicate a real reactivity bug.

```ts
// BAD: reads data.user during script init — not reactive, triggers warning
useServerRegistry(data.user);

// BAD: suppressing the warning hides the bug
// svelte-ignore state_referenced_locally
useServerRegistry(data.user);

// GOOD: getter defers the read to a reactive context ($effect)
useServerRegistry(() => data.user);
```

Inside the hook, call the getter within `$effect` for reactivity, and optionally once synchronously if downstream init code needs the value immediately (only if the called function is idempotent):

```ts
export function useServerRegistry(getUser: () => unknown): void {
  doSomething(!!getUser());        // Sync: immediate availability
  $effect(() => {
    doSomething(!!getUser());      // Reactive: responds to changes
  });
}
```

## Three-State Async Data: Render the Shell, Gate the Branches

For async data hooks like `useRoomData` that return `undefined` (loading) / `null` (not found) / object (loaded), prefer rendering the layout shell continuously and gating only the parts that genuinely require the loaded object. **Don't blanket-gate the whole tree on `{#if data}` just to keep children from mounting during loading** — that produces a blank flash on every prop change (e.g. switching rooms) before the skeleton can even appear.

`effect_update_depth_exceeded` is a runtime guard against effects that read **and** write the same state in a loop ([docs](https://svelte.dev/docs/svelte/runtime-errors#effect_update_depth_exceeded)). Many descendants computing fresh `$derived` values when async data finally arrives is normal Svelte 5 reactivity — Svelte batches it into a single run and it does not trip this error. If a particular effect does loop, fix the offender (typically by using `untrack` for self-writes, or replacing the effect with a `$derived`). Don't punish the rest of the tree with a top-level guard.

Pattern:

```svelte
{#if room.roomData !== null}
  <!-- Shell renders for both `undefined` (loading) and the loaded object. -->
  <PaneHeader title={title} loading={!room.roomData} />

  <!-- Components that work with just the URL params can stay mounted
       continuously across prop changes — their own loading state (e.g. a
       store's `isInitialLoading`) becomes the single skeleton across the
       transition, with no remount and no shimmer-phase reset. -->
  <RoomEventsPane {spaceId} {roomId} />

  <!-- Gate only what genuinely needs the loaded data. -->
  {#if room.roomData}
    <ThreadPane roomName={room.roomData.room.name} ... />
  {/if}
{/if}
```

The `null` branch is excluded because it triggers a redirect via `$effect.pre`; rendering during that brief window would flash the previous room's UI under the new (empty) data.

For the rare case where you actually do need to keep children unmounted during loading (e.g. a child's init code is destructive or expensive and the loading transition is short), a blanket `{#if data}` is acceptable — but treat it as the exception, not the default.

## Module-Level State Must Use `<script module>`

State that must survive component unmount/remount cycles (e.g., draft file stash maps, singleton caches) **must** be in `<script module>` — not in `<script>`. Instance-level `<script>` code runs on every mount, creating fresh state each time.

```svelte
<!-- Module-level: persists across component instances -->
<script module lang="ts">
  const draftFilesMap = new Map<string, FileWithUrl[]>();
</script>

<script lang="ts">
  // BAD: this creates a NEW map on every mount — stashed data is lost
  // const draftFilesMap = new Map<string, FileWithUrl[]>();
</script>
```

This matters when parent guards (`{#if data}`) cause child components to unmount during loading transitions and remount when data arrives. On the previous architecture where parent data was held via `$derived(await ...)`, components stayed mounted across transitions, hiding this class of bug.

## Snippets Are First-Class — Use `Snippet<[Args]>` to Share Layout

When two or three places in a component repeat the same wrapper-with-a-customized-body pattern (collapsible groups, list rows, conditional-content shells), factor the shell into a snippet that takes the body-renderer as a typed `Snippet<[Args]>` parameter. Pass it where you'd otherwise duplicate the wrapper.

```svelte
<script lang="ts">
  import type { Snippet } from 'svelte';
</script>

{#snippet collapsibleGroup(
  id: string,
  rooms: Room[],
  link: Snippet<[Room]>
)}
  {#each rooms as r (r.id)}
    {@render link(r)}
  {/each}
{/snippet}

{#snippet roomLink(room: Room)}<a href="...">{room.name}</a>{/snippet}
{#snippet dmLink(room: Room)}<a href="...">{dmDisplayName(room)}</a>{/snippet}

{@render collapsibleGroup('channels', channels, roomLink)}
{@render collapsibleGroup('dms', dmRooms, dmLink)}
```

This collapses four near-identical collapsible blocks in `RoomList.svelte` into one snippet — the "what counts as visible while collapsed" predicate (and the slide transition) lives in exactly one place, so channel and DM behaviour can't drift.

## Multi-Server Architecture

The frontend supports connecting to multiple Chatto servers simultaneously. The `ServerRegistry` (singleton at `serverRegistry`) owns both registration data and per-server state stores, ensuring atomic creation — no race between "server registered" and "store exists."

### URL is the Source of Truth

The URL determines which server is active:

- Landing page: `/` (welcome or redirect to Browse Spaces)
- Origin server: `/chat/-/SAbcDef/RAbcDef` (`-` = the server hosting the SPA)
- Remote server: `/chat/chat.hmans.dev/SAbcDef/RAbcDef` (hostname)

The `[serverId]/+layout.svelte` resolves the URL segment to a server ID via `segmentToServerId()` and provides it via Svelte context. Components inside this route tree use:

- `graphqlClientManager.getClient(serverId)` for the correct GraphQL client
- `serverRegistry.getStore(serverId)` for per-server state (notifications, permissions, etc.)
- `resolve()` from `$app/paths` for navigation (see Navigation section below)

**Key rule:** Never store "which server is active" in runtime state. Always derive it from the URL.

### No "Home Server" Flag

There is no `isHome` flag. The origin server (the server serving the SPA) is detected by comparing the registered server's `url` against `window.location.origin`:

- `serverRegistry.originServer` — finds the matching server (or `undefined`)
- `serverRegistry.isOriginInstance(id)` — checks a specific server (method name is vestigial; the concept is "origin server")

The origin uses cookie auth (`token: null`). Remote servers use bearer tokens (`token: string`).

### Origin Auto-Registration

On app init, `probeOrigin()` detects whether the SPA is served by a Chatto server by fetching `/api/server`. If it responds, the origin is auto-registered. If it fails (static hosting), nothing happens. This is idempotent — no-ops if already registered.

### Per-Server State

`ServerStateStore` is **the** home for state that's logically scoped to one server (rooms, notifications, room unread, room directory, current user, voice call, active call rooms, pending highlights, etc.). The registry instantiates one `ServerStateStore` per registered server — independent state per server, no synchronisation needed across server switches.

**Adding new per-server state:**

1. Implement the class without auto-loading in the constructor. Construction happens at registry registration, potentially before authentication; the substore stays empty and inert until something kicks it.
2. Add a `readonly` field to `ServerStateStore` and construct it in the constructor alongside the other per-server stores.
3. **Wire the substore's lifecycle from inside `ServerStateStore`'s constructor.** Use the bundle's `$effect.root` to:
   - call `refresh()` when `this.currentUser.user` flips truthy (handles bearer-auth load completion and cookie-auth post-login alike), and
   - forward live events from `eventBusManager.getBus(this.serverId)` into the substore via `ingestServerEvent` (or the equivalent method).

   See `ServerStateStore`'s constructor for the canonical shape. Doing this once, in the bundle, means every server keeps itself in sync with its own bus — the active server, an inactive-but-registered server, and a server you switch to an hour later all behave the same way.
4. Consumers read the substore via a reactive registry lookup, nothing else:

   ```ts
   const stores = $derived(serverRegistry.getStore(getServerId()));
   const rooms = $derived(stores.rooms);
   ```

   Reading `rooms.rooms`, `rooms.isInitialLoading`, etc. inside `$derived` / `$effect` / template expressions then re-evaluates against the *active* server's state automatically when the URL `[serverId]` param changes — no `{#key}`, no context refresh, no state sync, no page-level `$effect` for refresh, no `useEvent` for event ingestion.

**Anti-patterns (avoid):**

- Constructing a per-server store as a context singleton in a layout (`new MyStore(connection().client)` followed by `setMyStore(store)`). The layout stays mounted across `[serverId]` param changes — the store ends up frozen to the first server's client. Use `ServerStateStore` instead.
- Capturing `serverRegistry.getStore(id)` (or its fields) into a plain `const` in a long-lived component. After the URL changes, the captured reference still points at the previous server.
- Sprinkling `$effect(() => { void store.refresh(); })` or `useActiveEvent((event) => store.ingestServerEvent(event))` into every page that reads a per-server store. That's lifecycle work that belongs to the store; put it in `ServerStateStore`'s `$effect.root` once instead.

### Server-scoped contexts must be getters

A handful of contexts aren't state classes themselves but still depend on the active server — most notably the GraphQL client (`provideConnection` in `[serverId]/+layout.svelte`). The `[serverId]/+layout.svelte` stays mounted across `[serverId]` param changes, so a fixed value freezes descendants to whichever server happened to be active at the parent's one-time script init.

The fix: **pass a getter, not a value.** `useConnection` already follows this pattern:

```ts
// In the [serverId] layout
provideConnection(() => graphqlClientManager.getClient(serverId));

// In a consumer
const connection = useConnection(); // returns a getter
connection().client.query(...);     // resolves to the active server's client
```

For per-server *state* (current user, notifications, rooms, …), **don't reach for context at all** — those live on `ServerStateStore`. Consumers read them through the registry the same way they read any other per-server store:

```ts
const stores = $derived(serverRegistry.getStore(getServerId()));
const currentUser = $derived(stores.currentUser);
```

This keeps state lookups uniform across the codebase: one pattern, no exceptions. The historical "context that holds the active server's CurrentUserState" indirection was deleted along with the `getCurrentUser`/`setCurrentUser`/`initCurrentUser*` helpers; if you're tempted to add a similar layer for a new per-server state class, just add the field to `ServerStateStore` instead.

### Per-Server Permissions

Each `ServerStateStore` has a `permissions` field (`ServerPermissions`) loaded by `ServerSpaceSection` from the `viewer` query. Use `serverRegistry.getStore(id).permissions` to check per-server capabilities (e.g., `canCreateSpace`).

### Disconnect vs Sign Out

- **Disconnect** (`removeInstance`): Removes one server. Cleans up event bus, store, and GraphQL client. If it's the origin, also revokes the cookie session and hard-reloads.
- **Sign Out** (`removeAll`): Removes ALL servers, revokes origin cookie, hard-reloads to `/`.

### CORS Boundary

`/api/server` (REST) has wildcard CORS (`Access-Control-Allow-Origin: *`) — it's the **only** endpoint usable cross-origin without configuration. `/api/graphql` requires the client's origin in the allowed list. The add-server flow must use `/api/server` for probing remote servers. Rich server data (welcome message, config) should be included in `/api/server` when needed cross-origin, since GraphQL isn't accessible pre-registration.

## Use `createContext` for Svelte Context

Always use the `createContext` API from Svelte instead of manual `Symbol` keys with `getContext`/`setContext`. Re-export the `[get, set]` tuple directly — don't wrap them in functions unless the wrapper adds real logic (e.g., constructing the value, transforming the return):

```ts
import { createContext } from 'svelte';

// GOOD: re-export directly
export const [getMyContext, setMyContext] = createContext<MyType>();

// GOOD: factory wrapper that adds real logic (construction, options)
export const [getComposerContext, setComposerContext] = createContext<ComposerContext>();
export function createComposerContext(options?: Options): ComposerContext {
  const ctx = new ComposerContext(options);
  setComposerContext(ctx);
  return ctx;
}

// BAD: manual Symbol keys
const KEY = Symbol('myContext');
setContext(KEY, value);
getContext<MyType>(KEY);

// BAD: pointless wrapper around get/set
const [getCtx, setCtx] = createContext<MyType>();
export function getMyContext() { return getCtx(); }  // just re-export instead
```

## Prefer Context Over Mutable Singletons for URL-Derived State

When state is derived from the URL (route params), provide it via Svelte context from a layout — don't sync it into a mutable singleton via `$effect`.

```ts
// BAD: syncing URL param into mutable state creates timing bugs
$effect.pre(() => {
  registry.activeId = resolveFromUrl(page.params.serverId);
});
// Children may render before the effect runs, seeing stale state

// GOOD: context is available synchronously during child script init
const serverId = $derived(resolveFromUrl(page.params.serverId));
setActiveServer(() => serverId);
```

The `$effect`/`$effect.pre` approach fails because effects run after render — child components initialize with the old value and only see the update on the next tick. Context set during script initialization is available to children immediately.

## Navigation

**Always use `resolve()` from `$app/paths` for all internal navigation** — both `href` attributes and `goto()` calls. Use route IDs with params, not manually constructed path strings:

```ts
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';

// GOOD: type-safe route ID with params
resolve('/chat/[serverId]/[spaceId]/[roomId]', { serverId: serverSegment, spaceId, roomId })

// BAD: manual string construction
`/chat/${serverSegment}/${spaceId}/${roomId}`
```

`$lib/navigation` only exports two conversion functions: `serverIdToSegment(id)` (server ID → URL segment) and `segmentToServerId(segment)` (URL segment → server ID). There are no path builder helpers — use `resolve()` directly.

**DRY tip:** In components with 3+ resolve calls using the same server, derive the segment once:

```ts
const getServerId = getActiveServer();
const serverSegment = $derived(serverIdToSegment(getServerId()));
```
