<script module lang="ts">
  let scrollOffset = 700;

  export function setVirtualizerScrollOffset(offset: number) {
    scrollOffset = offset;
  }
</script>

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
  let scrollCalls = $state(0);

  export function scrollToIndex(index: number) {
    renderedIndex = index;
    scrollCalls += 1;
  }

  export function getScrollSize() {
    return 1_000;
  }

  export function getScrollOffset() {
    return scrollOffset;
  }

  export function getViewportSize() {
    return 300;
  }

  export function findItemIndex() {
    return 0;
  }
</script>

<output data-testid="virtualizer-scroll-index">{renderedIndex ?? ''}</output>
<output data-testid="virtualizer-scroll-calls">{scrollCalls}</output>
{#if renderedIndex !== null && data[renderedIndex] !== undefined}
  {@render children(data[renderedIndex])}
{/if}
