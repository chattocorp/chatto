<!--
@component

A virtualized list of collapsible sidebar groups. It renders the same headings,
dividers, and row spacing as stacked `RoomGroupSection`s, but mounts only the
headings and rows near the visible part of `scrollRef`. Use it for sidebar
lists that can grow large, such as room members and room files.

Collapsed state is shared with `RoomGroupSection` through `roomGroupCollapse`.
Rows do not animate in or out. Each mounted heading and row wrapper carries
`data-room-group-id`, so tests and callers can find the rows of one group.
-->
<script lang="ts" generics="T extends { id: string }">
  import type { Snippet } from 'svelte';
  import { Virtualizer } from 'virtua/svelte';
  import RoomGroupSectionHeader from './RoomGroupSectionHeader.svelte';
  import { loadCollapsed, saveCollapsed } from './roomGroupCollapse';
  import {
    buildGroupedListItems,
    type GroupedListItem,
    type VirtualListGroup
  } from './groupedListItems';

  interface Props {
    /** Groups in display order. Empty groups are the caller's choice to omit. */
    groups: readonly VirtualListGroup<T>[];
    /** Renders one row. */
    item: Snippet<[T]>;
    /** The scrolling ancestor. The list may share it with content that follows. */
    scrollRef: HTMLElement | undefined;
    /** Draw a divider above the first group too. */
    separateFirst?: boolean;
    /** Size hint in pixels for unmeasured rows, including row spacing. */
    itemSize?: number;
  }

  let { groups, item, scrollRef, separateFirst = false, itemSize }: Props = $props();

  const entries = $derived(
    buildGroupedListItems(
      groups.map((group) => ({
        ...group,
        collapsed: loadCollapsed(group.persistKey, group.defaultCollapsed ?? false)
      })),
      { separateFirst }
    )
  );
</script>

<!-- A flex parent must not shrink the virtualizer below its full scroll height. -->
<div class="shrink-0">
  <Virtualizer
    data={entries}
    getKey={(entry, index) => entry?.key ?? `__ix_${index}`}
    {scrollRef}
    {itemSize}
  >
    {#snippet children(entry: GroupedListItem<T>)}
      <!--
        virtua can re-run this snippet with a stale item while the data changes, so
        read every field through optional chaining before rendering it.
      -->
      {@const group = entry?.type === 'header' ? entry?.group : undefined}
      {@const row = entry?.type === 'row' ? entry?.row : undefined}
      {#if group && entry?.type === 'header'}
        <div
          class={[
            'px-2 pt-1.5',
            entry?.separated && 'border-t border-border',
            group.collapsed && 'pb-1.5'
          ]}
          data-room-group-id={group.id}
        >
          <RoomGroupSectionHeader
            label={group.label}
            collapsed={group.collapsed}
            ontoggle={() => saveCollapsed(group.persistKey, !group.collapsed)}
            testid={group.testid}
          />
        </div>
      {:else if row && entry?.type === 'row'}
        <div
          class={['px-2', entry?.last ? 'pb-1.5' : 'pb-0.5']}
          data-room-group-id={entry?.groupId}
        >
          {@render item(row)}
        </div>
      {/if}
    {/snippet}
  </Virtualizer>
</div>
