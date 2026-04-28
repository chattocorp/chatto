<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';

  let {
    items,
    columns,
    header,
    row,
    emptyMessage = 'No data',
    onRowClick,
    getKey,
    hoverable = true
  }: {
    items: T[];
    columns: number;
    header: Snippet;
    row: Snippet<[T]>;
    emptyMessage?: string;
    onRowClick?: (item: T) => void;
    getKey?: (item: T, index: number) => string | number;
    /**
     * Whether rows highlight on hover. Defaults to `true` for the standard
     * "list of records" treatment; pass `false` for matrix-style tables
     * where individual cells (not rows) are interactive and a row tint
     * would be visual noise.
     */
    hoverable?: boolean;
  } = $props();

  // Default key function: use id if present, otherwise use index
  function defaultGetKey(item: T, index: number): string | number {
    if (item && typeof item === 'object' && 'id' in item) {
      return (item as { id: string | number }).id;
    }
    return index;
  }

  const keyFn = $derived(getKey ?? defaultGetKey);
</script>

<table class="w-full">
  <thead>
    <tr class="border-b border-border text-left text-sm text-muted">
      {@render header()}
    </tr>
  </thead>
  <tbody>
    {#each items as item, index (keyFn(item, index))}
      <tr
        class={[
          'border-b border-border last:border-0',
          hoverable ? 'hover:bg-surface-200/40' : '',
          onRowClick ? 'cursor-pointer' : ''
        ]}
        onclick={() => onRowClick?.(item)}
      >
        {@render row(item)}
      </tr>
    {:else}
      <tr>
        <td colspan={columns} class="px-4 py-8 text-center text-muted">{emptyMessage}</td>
      </tr>
    {/each}
  </tbody>
</table>
