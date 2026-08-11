<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';

  let {
    items,
    header,
    row,
    emptyMessage,
    getKey = (item) => item,
    ..._unusedProps
  }: {
    items: T[];
    columns: number;
    header: Snippet;
    row: Snippet<[T]>;
    emptyMessage?: string;
    getKey?: (item: T, index: number) => unknown;
    hasMore?: boolean;
    loadingMore?: boolean;
    onLoadMore?: () => void | Promise<void>;
    loadMoreRoot?: HTMLElement;
    loadingMoreMessage?: string;
  } = $props();

</script>

<table>
  <thead>
    <tr>{@render header()}</tr>
  </thead>
  <tbody>
    {#each items as item, index (getKey(item, index))}
      <tr>{@render row(item)}</tr>
    {:else}
      <tr><td>{emptyMessage}</td></tr>
    {/each}
  </tbody>
</table>
