---
paths: ["frontend/**"]
---

# Frontend Development

## Svelte 5 Lifecycle Timing

`$effect` runs AFTER the initial render pass completes. Component script initialization (top-level `<script>` code) runs synchronously DURING render. This means:

- Parent script init → Parent template renders → **Child script init** → Child template renders → `onMount` (children first) → `$effect`

**Gotcha:** If a parent's `$effect` creates a resource (e.g., an event bus), child components cannot access it during their script initialization — it doesn't exist yet.

**Fix:** Create resources synchronously in the parent's script section for the initial render. Use `$effect` only for reactive changes after mount (additions/removals). See `+layout.svelte` bus initialization pattern:

```svelte
// Synchronous: available to children during render
for (const instance of instanceRegistry.instances) {
  instanceEventBusManager.startBus(instance.id, client);
}

// Reactive: handles additions/removals after initial render
$effect(() => {
  // startBus is idempotent — no-op if already started
  for (const instance of instanceRegistry.instances) {
    if (!instanceEventBusManager.getBus(instance.id)) {
      instanceEventBusManager.startBus(instance.id, client);
    }
  }
  return () => { /* cleanup */ };
});
```

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
useInstanceRegistry(data.user);

// BAD: suppressing the warning hides the bug
// svelte-ignore state_referenced_locally
useInstanceRegistry(data.user);

// GOOD: getter defers the read to a reactive context ($effect)
useInstanceRegistry(() => data.user);
```

Inside the hook, call the getter within `$effect` for reactivity, and optionally once synchronously if downstream init code needs the value immediately (only if the called function is idempotent):

```ts
export function useInstanceRegistry(getUser: () => unknown): void {
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

## Multi-Instance Architecture

The frontend supports connecting to multiple Chatto instances simultaneously. The `InstanceRegistry` (singleton at `instanceRegistry`) owns both registration data and per-instance state stores, ensuring atomic creation — no race between "instance registered" and "store exists."

### URL is the Source of Truth

The URL determines which instance is active:

- Landing page: `/` (welcome or redirect to Browse Spaces)
- Origin instance: `/chat/-/SAbcDef/RAbcDef` (`-` = the server hosting the SPA)
- Remote instance: `/chat/chat.hmans.dev/SAbcDef/RAbcDef` (hostname)

The `[instanceId]/+layout.svelte` resolves the URL segment to an instance ID via `segmentToInstanceId()` and provides it via Svelte context. Components inside this route tree use:

- `graphqlClientManager.getClient(instanceId)` for the correct GraphQL client
- `instanceRegistry.getStore(instanceId)` for per-instance state (notifications, permissions, etc.)
- `resolve()` from `$app/paths` for navigation (see Navigation section below)

**Key rule:** Never store "which instance is active" in runtime state. Always derive it from the URL.

### No "Home Instance" Flag

There is no `isHome` flag. The origin instance (the server serving the SPA) is detected by comparing `instance.url` against `window.location.origin`:

- `instanceRegistry.originInstance` — finds the matching instance (or `undefined`)
- `instanceRegistry.isOriginInstance(id)` — checks a specific instance

The origin uses cookie auth (`token: null`). Remote instances use bearer tokens (`token: string`).

### Origin Auto-Registration

On app init, `probeOrigin()` detects whether the SPA is served by a Chatto instance by fetching `/api/instance`. If it responds, the origin is auto-registered. If it fails (static hosting), nothing happens. This is idempotent — no-ops if already registered.

### Per-Instance Permissions

Each `InstanceStateStore` has a `permissions` field (`InstancePermissions`) loaded by `InstanceSpaceSection` from the `viewer` query. Use `instanceRegistry.getStore(id).permissions` to check per-instance capabilities (e.g., `canCreateSpace`).

### Disconnect vs Sign Out

- **Disconnect** (`removeInstance`): Removes one instance. Cleans up event bus, store, and GraphQL client. If it's the origin, also revokes the cookie session and hard-reloads.
- **Sign Out** (`removeAll`): Removes ALL instances, revokes origin cookie, hard-reloads to `/`.

### CORS Boundary

`/api/instance` (REST) has wildcard CORS (`Access-Control-Allow-Origin: *`) — it's the **only** endpoint usable cross-origin without configuration. `/api/graphql` requires the client's origin in the allowed list. The add-instance flow must use `/api/instance` for probing remote instances. Rich instance data (welcome message, config) should be included in `/api/instance` when needed cross-origin, since GraphQL isn't accessible pre-registration.

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
  registry.activeId = resolveFromUrl(page.params.instanceId);
});
// Children may render before the effect runs, seeing stale state

// GOOD: context is available synchronously during child script init
const instanceId = $derived(resolveFromUrl(page.params.instanceId));
setActiveInstance(() => instanceId);
```

The `$effect`/`$effect.pre` approach fails because effects run after render — child components initialize with the old value and only see the update on the next tick. Context set during script initialization is available to children immediately.

## Navigation

**Always use `resolve()` from `$app/paths` for all internal navigation** — both `href` attributes and `goto()` calls. Use route IDs with params, not manually constructed path strings:

```ts
import { resolve } from '$app/paths';
import { instanceIdToSegment } from '$lib/navigation';

// GOOD: type-safe route ID with params
resolve('/chat/[instanceId]/[spaceId]/[roomId]', { instanceId: instanceSegment, spaceId, roomId })

// BAD: manual string construction
`/chat/${instanceSegment}/${spaceId}/${roomId}`
```

`$lib/navigation` only exports two conversion functions: `instanceIdToSegment(id)` (instance ID → URL segment) and `segmentToInstanceId(segment)` (URL segment → instance ID). There are no path builder helpers — use `resolve()` directly.

**DRY tip:** In components with 3+ resolve calls using the same instance, derive the segment once:

```ts
const getInstanceId = getActiveInstance();
const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
```

**DM routes use `[instanceSegment]`**, not `[instanceId]`:

```ts
resolve('/chat/dm/[instanceSegment]/[conversationId]', { instanceSegment, conversationId })
```
