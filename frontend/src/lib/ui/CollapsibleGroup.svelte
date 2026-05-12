<!--
@component

A sidebar group with a collapsible header. Collapsed/expanded state is
persisted to `localStorage` under `persistKey`; callers needing per-server
scoping should build the key with `serverStorageKey()`.

When collapsed, items are hidden unless `keepVisibleWhenCollapsed` returns
true for them — useful for anchoring rows that demand attention (active,
unread, mentions, …) so the user can always reach them.

Used by `RoomList` (channels, DMs, layout sections) and `RoomInfo` (online /
offline member groups).
-->
<script module lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { Codecs, StorageSlot } from '$lib/storage/slot';

  // Module-level reactive cache, write-through to localStorage. Groups
  // that share a `persistKey` stay in sync automatically (no shared key
  // pairs exist today — this just falls out of the pattern).
  const cache = new SvelteMap<string, boolean>();

  function loadCollapsed(key: string, fallback: boolean): boolean {
    const cached = cache.get(key);
    if (cached !== undefined) return cached;
    return new StorageSlot(key, fallback, Codecs.boolean).get();
  }

  function saveCollapsed(key: string, value: boolean): void {
    cache.set(key, value);
    new StorageSlot(key, value, Codecs.boolean).set(value);
  }
</script>

<script lang="ts" generics="T extends { id: string }">
  import type { Snippet } from 'svelte';
  import { slide } from 'svelte/transition';

  interface Props {
    label: string;
    items: T[];
    item: Snippet<[T]>;
    /** Unique localStorage key for persisting collapsed state. */
    persistKey: string;
    /** Collapsed state when no preference is stored. */
    defaultCollapsed?: boolean;
    keepVisibleWhenCollapsed?: (item: T) => boolean;
    class?: string;
  }

  let {
    label,
    items,
    item,
    persistKey,
    defaultCollapsed = false,
    keepVisibleWhenCollapsed,
    class: className
  }: Props = $props();

  const collapsed = $derived(loadCollapsed(persistKey, defaultCollapsed));

  function toggle() {
    saveCollapsed(persistKey, !collapsed);
  }
</script>

<div class={className}>
  <button
    type="button"
    onclick={toggle}
    class="flex w-full cursor-pointer items-center gap-2 px-3 py-1 text-xs font-semibold tracking-wider text-muted uppercase transition-colors hover:text-text"
  >
    <span class="sidebar-icon">
      <span
        class={['iconify uil--angle-right-b transition-transform', collapsed ? '' : 'rotate-90']}
      ></span>
    </span>
    {label}
  </button>
  <div class="sidebar-nav">
    {#each items as it (it.id)}
      {#if !collapsed || keepVisibleWhenCollapsed?.(it)}
        <div transition:slide={{ duration: 150 }}>
          {@render item(it)}
        </div>
      {/if}
    {/each}
  </div>
</div>
