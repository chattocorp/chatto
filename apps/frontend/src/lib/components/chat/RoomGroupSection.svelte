<!--
@component

A persistent, collapsible room-group section for the server sidebar. Collapsing
a section hides ordinary entries while keeping caller-identified attention rows
visible, so active rooms, calls, unread rooms, and notifications remain
reachable.
-->
<script module lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { Codecs, StorageSlot } from '$lib/storage/slot';

  const collapsedByKey = new SvelteMap<string, boolean>();

  function loadCollapsed(key: string, fallback: boolean): boolean {
    const cached = collapsedByKey.get(key);
    if (cached !== undefined) return cached;
    return new StorageSlot(key, fallback, Codecs.boolean).get();
  }

  function saveCollapsed(key: string, value: boolean): void {
    collapsedByKey.set(key, value);
    new StorageSlot(key, value, Codecs.boolean).set(value);
  }
</script>

<script lang="ts" generics="T extends { id: string }">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { slide } from 'svelte/transition';
  import { COMPACT_MOTION_DURATION_MS, expoOutTransition } from '$lib/ui/motion';

  interface Props {
    label: string;
    items: T[];
    item: Snippet<[T]>;
    /** Whether to draw the full-width divider preceding this section. */
    separated?: boolean;
    /** Optional right-click/long-press behavior for the group header. */
    contextMenuTrigger?: Attachment<HTMLElement>;
    /** Unique localStorage key for persisting collapsed state. */
    persistKey: string;
    /** Collapsed state when no preference is stored. */
    defaultCollapsed?: boolean;
    keepVisibleWhenCollapsed?: (item: T) => boolean;
  }

  let {
    label,
    items,
    item,
    separated = false,
    contextMenuTrigger,
    persistKey,
    defaultCollapsed = false,
    keepVisibleWhenCollapsed
  }: Props = $props();

  const collapsed = $derived(loadCollapsed(persistKey, defaultCollapsed));
  const visibleItems = $derived(
    collapsed ? items.filter((entry) => keepVisibleWhenCollapsed?.(entry) ?? false) : items
  );

  function toggle(): void {
    saveCollapsed(persistKey, !collapsed);
  }
</script>

<section
  class={separated ? 'border-t border-border' : undefined}
  data-testid="room-group-section"
>
  <div class="px-2 py-1.5">
    <button
      type="button"
      onclick={toggle}
      aria-expanded={!collapsed}
      class="flex min-h-8 w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-start text-xs font-semibold tracking-wider text-muted uppercase transition-colors hover:text-text"
      {@attach contextMenuTrigger}
    >
      <span class="sidebar-icon">
        <span
          class={[
            'iconify transition-transform icon-[uil--angle-right-b]',
            collapsed ? 'rtl:-scale-x-100' : 'rotate-90'
          ]}
          aria-hidden="true"
        ></span>
      </span>
      <span class="min-w-0 flex-1 truncate">{label}</span>
    </button>

    {#if visibleItems.length > 0}
      <div class="flex flex-col gap-0.5">
        {#each visibleItems as entry (entry.id)}
          <div transition:slide={expoOutTransition(COMPACT_MOTION_DURATION_MS)}>
            {@render item(entry)}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>
