<!--
@component

A sidebar group with a collapsible header. The header is a button that toggles
the `collapsed` state via `onToggle`; collapsed/expanded state is owned by the
caller so it can persist (e.g. to localStorage).

When collapsed, items are hidden unless `keepVisibleWhenCollapsed` returns true
for them — useful for anchoring rows that demand attention (active, unread,
mentions, …) so the user can always reach them.

Used by `RoomList` (channels, DMs, layout sections) and `RoomInfo` (online /
offline member groups).
-->
<script lang="ts" generics="T extends { id: string }">
  import type { Snippet } from 'svelte';
  import { slide } from 'svelte/transition';

  interface Props {
    label: string;
    items: T[];
    item: Snippet<[T]>;
    collapsed: boolean;
    onToggle: () => void;
    keepVisibleWhenCollapsed?: (item: T) => boolean;
    class?: string;
  }

  let {
    label,
    items,
    item,
    collapsed,
    onToggle,
    keepVisibleWhenCollapsed,
    class: className
  }: Props = $props();
</script>

<div class={className}>
  <button
    type="button"
    onclick={onToggle}
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
