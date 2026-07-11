<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    data,
    children
  }: {
    data: unknown[];
    children: Snippet<[unknown]>;
  } = $props();

  let renderedIndex = $state<number | null>(null);

  export function scrollToIndex(index: number) {
    renderedIndex = index;
  }

  export function getScrollSize() {
    return 1_000;
  }

  export function getScrollOffset() {
    return 400;
  }

  export function getViewportSize() {
    return 300;
  }

  export function findItemIndex() {
    return 0;
  }
</script>

<output data-testid="virtualizer-scroll-index">{renderedIndex ?? ''}</output>
{#if renderedIndex !== null && data[renderedIndex] !== undefined}
  {@render children(data[renderedIndex])}
{/if}
