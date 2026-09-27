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
  import { tick, untrack, type Snippet } from 'svelte';
  import { on } from 'svelte/events';
  import { Virtualizer, type VirtualizerHandle } from 'virtua/svelte';
  import RoomGroupSectionHeader from './RoomGroupSectionHeader.svelte';
  import { loadCollapsed, saveCollapsed } from './roomGroupCollapse';
  import {
    buildGroupedListItems,
    type GroupedListItem,
    type VirtualListGroup
  } from './groupedListItems';
  import {
    ANCHOR_KEY_ATTRIBUTE,
    alignScrollAnchor,
    captureScrollAnchors,
    selectScrollAnchor,
    type ScrollAnchor
  } from './groupedListScrollAnchor';

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

  let virtualizer = $state<VirtualizerHandle>();
  let listElement = $state<HTMLDivElement>();
  // Entries as last rendered, and the entries visible in that layout. Anchors are recorded
  // after scrolling and after each layout settles, because by the time `entries` changes
  // the virtualizer has already rendered the new entries.
  let renderedEntries: GroupedListItem<T>[] = [];
  let anchors: ScrollAnchor[] = [];

  function recordAnchors(): void {
    anchors =
      listElement && scrollRef ? captureScrollAnchors(listElement, scrollRef, renderedEntries) : [];
  }

  // The virtualizer renders the entries for a new scroll position after the scroll event.
  $effect(() => {
    const scroller = scrollRef;
    if (!scroller) return;
    return on(scroller, 'scroll', () => void tick().then(recordAnchors), { passive: true });
  });

  // Keep the visible entries in place when entries above them are inserted, removed, or
  // moved to another group, as browser scroll anchoring did for the stacked sections.
  $effect(() => {
    const next = entries;
    return untrack(() => {
      renderedEntries = next;
      const selected = selectScrollAnchor(next, anchors);
      const handle = virtualizer;
      const list = listElement;
      const scroller = scrollRef;
      const align =
        selected && handle && list && scroller
          ? () => alignScrollAnchor(handle, list, scroller, selected.anchor, selected.index)
          : () => {};
      align();

      // The virtualizer measures moved entries after this update and corrects the scroll
      // position for size changes above the viewport. Align again once that has settled,
      // then record the anchors of the settled layout.
      let frame = 0;
      let remaining = 3;
      const settle = () => {
        align();
        remaining -= 1;
        if (remaining > 0) {
          frame = requestAnimationFrame(settle);
        } else {
          frame = 0;
          recordAnchors();
        }
      };
      frame = requestAnimationFrame(settle);
      return () => cancelAnimationFrame(frame);
    });
  });
</script>

<!-- A flex parent must not shrink the virtualizer below its full scroll height. -->
<div class="shrink-0" bind:this={listElement}>
  <Virtualizer
    bind:this={virtualizer}
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
          {...{ [ANCHOR_KEY_ATTRIBUTE]: entry?.key }}
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
          {...{ [ANCHOR_KEY_ATTRIBUTE]: entry?.key }}
        >
          {@render item(row)}
        </div>
      {/if}
    {/snippet}
  </Virtualizer>
</div>
