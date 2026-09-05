<!--
@component

Wraps a scrollable region with edge fade overlays. Provides a
`position: relative` outer wrapper containing an inner overflow-y-auto
scroll container; children render inside the scroll container.

- The fades hide automatically when the scroll is at the matching edge.
- The scroll element is exposed via `bind:scrollEl` so callers can wire
  things that need it (virtua `scrollRef`, scroll-to-bottom logic,
  etc.).
- A `refresh()` component method is exposed via `bind:this` for callers
  that make external layout changes and need edge re-measurement.
- Extra props (e.g. `data-testid`, `onwheel`, `ontouchmove`) are
  forwarded to the scroll container.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import ScrollArea from './ScrollArea.svelte';
  import { readScrollEdges, trackScrollEdges } from './scrollEdges';

  type Props = {
    children: Snippet;
    /** Show the top fade overlay. */
    top?: boolean;
    /** Show the bottom fade overlay. */
    bottom?: boolean;
    /** Tailwind class for fade height. Default `h-8`. */
    fadeHeight?: string;
    /** Tailwind position class for the top fade. Default `top-0`. */
    topFadeOffset?: string;
    /** Let the inner viewport scroll horizontally as well as vertically. */
    scrollX?: boolean;
    /** Fill the remaining height of a flex parent. Disable for intrinsic-height viewports. */
    fill?: boolean;
    /** Tailwind classes for the opaque end of each fade. */
    fadeColorClass?: string;
    /** Extra classes for the outer positioning wrapper. */
    class?: string;
    /** Extra classes for the inner scroll container. */
    scrollClass?: string;
    /** Bound to the inner scroll container so callers can reference it. */
    scrollEl?: HTMLDivElement;
    /** Keep the scroll viewport in the tab order for keyboard scrolling. */
    keyboardFocusable?: boolean;
    [key: string]: unknown;
  };

  let {
    children,
    top = false,
    bottom = false,
    fadeHeight = 'h-8',
    topFadeOffset = 'top-0',
    scrollX = false,
    fill = true,
    fadeColorClass = 'from-background',
    class: className = '',
    scrollClass = '',
    scrollEl = $bindable(),
    keyboardFocusable = true,
    ...rest
  }: Props = $props();

  let scrolledFromTop = $state(false);
  let scrolledFromBottom = $state(false);

  function setScrollEdges(edges: { start: boolean; end: boolean }) {
    scrolledFromTop = edges.start;
    scrolledFromBottom = edges.end;
  }

  const observeScrollEdges = trackScrollEdges('y', setScrollEdges);

  export function refresh() {
    if (!scrollEl) return;

    requestAnimationFrame(() => {
      if (scrollEl) setScrollEdges(readScrollEdges(scrollEl, 'y'));
    });
  }
</script>

{#snippet fades()}
  {#if top}
    <div
      aria-hidden="true"
      class={[
        'pointer-events-none absolute inset-x-0 z-30 bg-gradient-to-b to-transparent transition-opacity',
        fadeColorClass,
        topFadeOffset,
        fadeHeight,
        !scrolledFromTop && 'opacity-0'
      ]}
    ></div>
  {/if}
  {#if bottom}
    <div
      aria-hidden="true"
      class={[
        'pointer-events-none absolute inset-x-0 bottom-0 z-30 bg-gradient-to-t to-transparent transition-opacity',
        fadeColorClass,
        fadeHeight,
        !scrolledFromBottom && 'opacity-0'
      ]}
    ></div>
  {/if}
{/snippet}

<ScrollArea
  {scrollX}
  {fill}
  class={className}
  {scrollClass}
  bind:scrollEl
  {keyboardFocusable}
  scrollAttachment={observeScrollEdges}
  overlay={fades}
  {...rest}
>
    {@render children()}
</ScrollArea>
