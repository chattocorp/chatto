<script module lang="ts">
  let scrollOffset = 700;
  let forcedRenderedIndex: number | null = null;

  export function setVirtualizerScrollOffset(offset: number) {
    scrollOffset = offset;
  }

  export function setVirtualizerForcedRenderedIndex(index: number | null) {
    forcedRenderedIndex = index;
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

  let renderedIndex = $state<number | null>(forcedRenderedIndex);
  let scrollCalls = $state(0);
  let lastAlignment = $state('');
  let renderedItem = $derived(renderedIndex === null ? undefined : data[renderedIndex]);
  let renderedKey = $derived(
    (renderedItem as { key?: string } | undefined)?.key ?? renderedIndex ?? 'empty'
  );

  export function scrollToIndex(index: number, options?: { align?: string }) {
    renderedIndex = forcedRenderedIndex ?? index;
    scrollCalls += 1;
    lastAlignment = options?.align ?? '';
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
<output data-testid="virtualizer-scroll-alignment">{lastAlignment}</output>
<output data-testid="virtualizer-rendered-key" data-rendered-key={renderedKey}></output>
{#if renderedItem !== undefined}
  {#key renderedKey}
    {@render children(renderedItem)}
  {/key}
{/if}
