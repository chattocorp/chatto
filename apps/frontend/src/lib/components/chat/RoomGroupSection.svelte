<!--
@component

A persistent, collapsible section for Chatto sidebars. It provides the shared
heading, full-width divider, item spacing, and disclosure behaviour used by room
navigation, member presence groups, and attachment date groups. Collection
transitions belong to the outer conditional block so an empty collapsed group
can slide out before its rows are removed.
-->
<script lang="ts" generics="T extends { id: string }">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { flip } from 'svelte/animate';
  import { SHADOW_ITEM_MARKER_PROPERTY_NAME, SHADOW_PLACEHOLDER_ITEM_ID } from 'svelte-dnd-action';
  import { slide } from 'svelte/transition';
  import { COMPACT_MOTION_DURATION_MS, expoOutTransition } from '$lib/ui/motion';
  import RoomGroupSectionHeader from './RoomGroupSectionHeader.svelte';
  import { loadCollapsed, saveCollapsed } from './roomGroupCollapse';

  interface Props {
    label: string;
    items: T[];
    item?: Snippet<[T]>;
    /** Free-form content stays mounted while collapsed so async rendering cannot interrupt expansion. */
    content?: Snippet;
    /** Optional controls aligned to the end of the section heading. */
    headerActions?: Snippet;
    /** Optional content below the items, visible only while expanded. */
    footer?: Snippet;
    /** Optional action that replaces the disclosure icon on hover or focus. */
    leadingOverlay?: Snippet;
    /** Whether to draw the full-width divider preceding this section. */
    separated?: boolean;
    /** Optional right-click/long-press behavior for the group header. */
    contextMenuTrigger?: Attachment<HTMLElement>;
    /** Optional behavior attached to the rendered item collection. */
    itemsAttachment?: Attachment<HTMLDivElement>;
    /** Prevent item drags from also reaching a containing section drag zone. */
    containItemDrag?: boolean;
    /** Whether this section is the temporary shadow for a containing drag zone. */
    isDndShadow?: boolean;
    /** Unique localStorage key for persisting collapsed state. */
    persistKey: string;
    /** Collapsed state when no preference is stored. */
    defaultCollapsed?: boolean;
    keepVisibleWhenCollapsed?: (item: T) => boolean;
    /** Optional stable selector for the disclosure button. */
    testid?: string;
  }

  let {
    label,
    items,
    item,
    content,
    headerActions,
    footer,
    leadingOverlay,
    separated = false,
    contextMenuTrigger,
    itemsAttachment,
    containItemDrag = false,
    isDndShadow = false,
    persistKey,
    defaultCollapsed = false,
    keepVisibleWhenCollapsed,
    testid
  }: Props = $props();

  const collapsed = $derived(loadCollapsed(persistKey, defaultCollapsed));
  const visibleItems = $derived(
    collapsed ? items.filter((entry) => keepVisibleWhenCollapsed?.(entry) ?? false) : items
  );

  function toggle(): void {
    saveCollapsed(persistKey, !collapsed);
  }

  function isDndShadowItem(item: T): boolean {
    const dndItem = item as T & Record<string, unknown>;
    return (
      item.id === SHADOW_PLACEHOLDER_ITEM_ID || dndItem[SHADOW_ITEM_MARKER_PROPERTY_NAME] === true
    );
  }

  function containNestedDrag(event: MouseEvent | TouchEvent): void {
    if (!containItemDrag) return;
    const target = event.target;
    if (target instanceof Element && target.closest('[data-room-group-drag-handle]')) return;
    event.stopPropagation();
  }

  const containNestedDragAttachment: Attachment<HTMLElement> = (node) => {
    node.addEventListener('mousedown', containNestedDrag);
    node.addEventListener('touchstart', containNestedDrag);
    return () => {
      node.removeEventListener('mousedown', containNestedDrag);
      node.removeEventListener('touchstart', containNestedDrag);
    };
  };
</script>

<section
  class={[separated ? 'border-t border-border' : '', isDndShadow ? 'rounded-md' : '']}
  data-is-dnd-shadow-item-hint={isDndShadow || undefined}
  data-testid="room-group-section"
  {@attach containNestedDragAttachment}
>
  <div class="px-2 py-1.5">
    <RoomGroupSectionHeader
      {label}
      {collapsed}
      ontoggle={toggle}
      {headerActions}
      {leadingOverlay}
      {contextMenuTrigger}
      {testid}
    />

    {#if content}
      <div
        class={[
          'grid transition-[grid-template-rows] duration-180 ease-out motion-reduce:transition-none',
          collapsed ? 'grid-rows-[0fr]' : 'grid-rows-[1fr]'
        ]}
        inert={collapsed}
      >
        <div class="min-h-0 overflow-hidden">
          {@render content()}
        </div>
      </div>
    {/if}

    {#if visibleItems.length > 0 || (itemsAttachment && !collapsed)}
      <!-- Update individual classes so the drag attachment keeps its active highlight. -->
      <div
        class="flex flex-col gap-0.5"
        class:min-h-8={visibleItems.length === 0}
        class:sidebar-drop-target={!!itemsAttachment}
        data-testid={itemsAttachment ? 'room-group-items-dropzone' : undefined}
        {@attach itemsAttachment}
        transition:slide={expoOutTransition(COMPACT_MOTION_DURATION_MS)}
      >
        {#if itemsAttachment}
          {#each visibleItems as entry (entry.id)}
            <div
              animate:flip={expoOutTransition(COMPACT_MOTION_DURATION_MS)}
              transition:slide={expoOutTransition(COMPACT_MOTION_DURATION_MS)}
              data-is-dnd-shadow-item-hint={isDndShadowItem(entry) || undefined}
            >
              {@render item?.(entry)}
            </div>
          {/each}
        {:else}
          {#each visibleItems as entry (entry.id)}
            <div transition:slide={expoOutTransition(COMPACT_MOTION_DURATION_MS)}>
              {@render item?.(entry)}
            </div>
          {/each}
        {/if}
      </div>
    {/if}
    {#if !collapsed && footer}
      <div transition:slide={expoOutTransition(COMPACT_MOTION_DURATION_MS)}>
        {@render footer()}
      </div>
    {/if}
  </div>
</section>
