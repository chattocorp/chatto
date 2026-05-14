<!--
@component

Generic autocomplete popup with keyboard navigation.
Renders a positioned menu above the composer input with arrow key navigation,
configurable selection keys, and customizable item rendering via snippets.

**Props:**
- `items` - Array of items to display
- `getKey` - Function to extract a unique key from each item
- `selectKeys` - Which keys trigger selection (default: Enter and Tab)
- `onSelect` - Callback when an item is selected (receives item and the key used)
- `onClose` - Callback to close the popup
- `testid` - Optional data-testid for e2e tests
- `class` - Additional classes for the container
- `item` - Snippet to render each item (receives { item, selected })
-->
<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';

  type Props = {
    items: T[];
    getKey: (item: T) => string;
    selectKeys?: string[];
    onSelect: (item: T, key: string) => void;
    onClose: () => void;
    testid?: string;
    class?: string;
    item: Snippet<[{ item: T; selected: boolean }]>;
  };

  let {
    items,
    getKey,
    selectKeys = ['Enter', 'Tab'],
    onSelect,
    onClose,
    testid,
    class: className = '',
    item
  }: Props = $props();

  let selectedIndex = $state(0);

  // Reset selection when items change
  $effect(() => {
    void items;
    selectedIndex = 0;
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
  <div
    data-testid={testid}
    class="absolute bottom-full left-0 z-50 mb-2 max-h-80 w-full overflow-y-auto menu {className}"
  >
    <ul class="menu-section">
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
{/if}
