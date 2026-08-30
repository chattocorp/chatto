<!--
@component

A persistent, collapsible section for Chatto sidebars. It provides the shared
heading, full-width divider, item spacing, and disclosure behaviour used by room
navigation, member presence groups, and attachment date groups.
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
  import { flip } from 'svelte/animate';
  import { SHADOW_ITEM_MARKER_PROPERTY_NAME, SHADOW_PLACEHOLDER_ITEM_ID } from 'svelte-dnd-action';
  import { slide } from 'svelte/transition';
  import { COMPACT_MOTION_DURATION_MS, expoOutTransition } from '$lib/ui/motion';

  interface Props {
    label: string;
    items: T[];
    item: Snippet<[T]>;
    /** Optional controls aligned to the end of the section heading. */
    headerActions?: Snippet;
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
    headerActions,
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
    <div
      class="group/section-header relative flex min-h-8 w-full min-w-0 items-center rounded-md transition-colors hover:text-text"
      {@attach contextMenuTrigger}
    >
      <button
        type="button"
        onclick={toggle}
        aria-expanded={!collapsed}
        data-testid={testid}
        class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-start text-xs font-semibold tracking-wider text-muted uppercase focus-visible:outline-2 focus-visible:outline-action"
      >
        <span class="relative sidebar-icon">
          <span
            class={[
              'iconify icon-[uil--angle-right-b] transition-[transform,opacity]',
              leadingOverlay
                ? 'group-focus-within/section-header:opacity-0 group-hover/section-header:opacity-0 [@media(hover:none)]:opacity-0'
                : '',
              collapsed ? 'rtl:-scale-x-100' : 'rotate-90'
            ]}
            aria-hidden="true"
            data-testid="room-group-disclosure-icon"
          ></span>
        </span>
        <span class="min-w-0 flex-1 truncate">{label}</span>
      </button>
      {#if leadingOverlay}
        <span class="pointer-events-none absolute start-0.5 top-1 h-6 w-6">
          {@render leadingOverlay()}
        </span>
      {/if}
      {#if headerActions}
        <div class="flex shrink-0 items-center gap-0.5">
          {@render headerActions()}
        </div>
      {/if}
    </div>

    {#if visibleItems.length > 0 || (itemsAttachment && !collapsed)}
      <div
        class={['flex flex-col gap-0.5', visibleItems.length === 0 ? 'min-h-8' : '']}
        data-testid={itemsAttachment ? 'room-group-items-dropzone' : undefined}
        {@attach itemsAttachment}
      >
        {#if itemsAttachment}
          {#each visibleItems as entry (entry.id)}
            <div
              animate:flip={{ duration: COMPACT_MOTION_DURATION_MS }}
              data-is-dnd-shadow-item-hint={isDndShadowItem(entry) || undefined}
            >
              {@render item(entry)}
            </div>
          {/each}
        {:else}
          {#each visibleItems as entry (entry.id)}
            <div transition:slide={expoOutTransition(COMPACT_MOTION_DURATION_MS)}>
              {@render item(entry)}
            </div>
          {/each}
        {/if}
      </div>
    {/if}
  </div>
</section>
