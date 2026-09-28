<!--
@component

Generic autocomplete popup with keyboard navigation.
Renders a menu above the composer input with arrow key navigation,
configurable selection keys, and customizable item rendering via snippets.

Place it inside the positioned composer surface. An invisible marker covers
that surface, and `FloatingPopover` anchors the menu to the marker in the top
layer, so scroll containers and stacking contexts cannot clip it. The popover
never takes focus: the editor keeps focus and forwards keys to `handleKeyDown`.

**Props:**
- `items` - Array of items to display
- `getKey` - Function to extract a unique key from each item
- `selectKeys` - Which keys trigger selection (default: Enter and Tab)
- `onSelect` - Callback when an item is selected (receives item and the key used)
- `onClose` - Callback to close the popup
- `testid` - Optional data-testid for e2e tests
- `class` - Additional classes for the menu, such as a wider-screen width
- `item` - Snippet to render each item (receives { item, selected })
-->
<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import { on } from 'svelte/events';
  import { FloatingPopover } from '$lib/ui';

  type Props = {
    items: T[];
    getKey: (item: T) => string;
    selectKeys?: string[];
    onSelect: (item: T, key: string) => void;
    onClose: () => void;
    testid?: string;
    class?: ClassValue;
    item: Snippet<[{ item: T; selected: boolean }]>;
  };

  let {
    items,
    getKey,
    selectKeys = ['Enter', 'Tab'],
    onSelect,
    onClose,
    testid,
    class: className,
    item
  }: Props = $props();

  /** Viewport rect of the composer surface that the menu opens above. */
  let anchor = $state<{ top: number; bottom: number; left: number; width: number } | null>(null);

  // Track the surface rect while the menu is visible. The composer grows with
  // its draft, and the visual viewport shrinks when a touch keyboard opens.
  function trackAnchor(marker: HTMLElement) {
    const update = () => {
      const rect = marker.getBoundingClientRect();
      anchor = { top: rect.top, bottom: rect.bottom, left: rect.left, width: rect.width };
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(marker);
    const offWindowResize = on(window, 'resize', update);
    const offScroll = on(window, 'scroll', update, { capture: true, passive: true });
    const offViewportResize = window.visualViewport
      ? on(window.visualViewport, 'resize', update)
      : () => {};
    return () => {
      observer.disconnect();
      offWindowResize();
      offScroll();
      offViewportResize();
      // Measure again when the menu reopens instead of reusing a stale rect.
      anchor = null;
    };
  }

  let selectedIndex = $derived.by(() => {
    void items;
    return 0;
  });

  export function handleKeyDown(event: KeyboardEvent): boolean {
    if (items.length === 0) return false;

    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        selectedIndex = (selectedIndex + 1) % items.length;
        return true;
      case 'ArrowUp':
        event.preventDefault();
        selectedIndex = (selectedIndex - 1 + items.length) % items.length;
        return true;
      case 'Escape':
        event.preventDefault();
        onClose();
        return true;
      default:
        if (selectKeys.includes(event.key)) {
          event.preventDefault();
          onSelect(items[selectedIndex], event.key);
          return true;
        }
        return false;
    }
  }
</script>

{#if items.length > 0}
  <div class="pointer-events-none absolute inset-0" aria-hidden="true" {@attach trackAnchor}></div>
  {#if anchor}
    <FloatingPopover {anchor} anchorPlacement="top">
      <div
        data-testid={testid}
        class={['flex max-h-80 w-[var(--autocomplete-anchor-width)] flex-col menu', className]}
        style:--autocomplete-anchor-width={`${anchor.width}px`}
      >
        <!-- Scroll inside the section so the frame's inset and border stay visible. -->
        <ul class="min-h-0 overflow-y-auto menu-section">
          {#each items as entry, index (getKey(entry))}
            <li>
              <button
                type="button"
                class={['menu-item', index === selectedIndex && 'menu-item-active']}
                onmouseenter={() => (selectedIndex = index)}
                onclick={() => onSelect(entry, 'click')}
                {@attach (el) => {
                  // Keep the selected item in view during keyboard navigation
                  if (index === selectedIndex) el.scrollIntoView({ block: 'nearest' });
                }}
              >
                {@render item({ item: entry, selected: index === selectedIndex })}
              </button>
            </li>
          {/each}
        </ul>
      </div>
    </FloatingPopover>
  {/if}
{/if}
