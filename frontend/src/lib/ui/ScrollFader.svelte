<!--
@component

Wraps a scrollable region with edge fade overlays. Provides a
`position: relative` outer wrapper containing an inner overflow-y-auto
scroll container; children render inside the scroll container.

- The fades hide automatically when the scroll is at the matching edge.
- The scroll element is exposed via `bind:scrollEl` so callers can wire
  things that need it (virtua `scrollRef`, scroll-to-bottom logic,
  etc.).
- Extra props (e.g. `data-testid`, `onwheel`, `ontouchmove`) are
  forwarded to the scroll container.

Use this when a scrollable region should fade content into the
boundaries adjacent to it (header, footer, composer, …).
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import ScrollFade from './ScrollFade.svelte';

  type Props = {
    children: Snippet;
    /** Show the top fade overlay. */
    top?: boolean;
    /** Show the bottom fade overlay. */
    bottom?: boolean;
    /** Tailwind class for fade height. Default `h-8`. */
    fadeHeight?: string;
    /** Extra classes for the outer positioning wrapper. */
    class?: string;
    /** Extra classes for the inner scroll container. */
    scrollClass?: string;
    /** Bound to the inner scroll container so callers can reference it. */
    scrollEl?: HTMLDivElement;
    [key: string]: unknown;
  };

  let {
    children,
    top = false,
    bottom = false,
    fadeHeight = 'h-8',
    class: className = '',
    scrollClass = '',
    scrollEl = $bindable(),
    ...rest
  }: Props = $props();
</script>

<div class={['relative flex min-h-0 min-w-0 flex-1 flex-col', className]}>
  <div
    bind:this={scrollEl}
    class={[
      'flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto',
      scrollClass
    ]}
    {...rest}
  >
    {@render children()}
  </div>
  {#if top}
    <ScrollFade target={() => scrollEl} edge="top" height={fadeHeight} />
  {/if}
  {#if bottom}
    <ScrollFade target={() => scrollEl} edge="bottom" height={fadeHeight} />
  {/if}
</div>
