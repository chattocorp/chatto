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
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

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
    /** Bump when external layout work should force edge re-measurement. */
    refreshKey?: unknown;
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
    refreshKey,
    ...rest
  }: Props = $props();

  let scrolledFromTop = $state(false);
  let scrolledFromBottom = $state(false);

  function updateScrollEdges(el: HTMLElement) {
    scrolledFromTop = el.scrollTop > 1;
    scrolledFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight > 1;
  }

  function trackScrollEdges(el: HTMLElement) {
    const update = () => {
      updateScrollEdges(el);
    };

    update();
    el.addEventListener('scroll', update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', update);
      ro.disconnect();
    };
  }

  $effect(() => {
    void refreshKey;
    if (!scrollEl) return;

    const frame = requestAnimationFrame(() => {
      if (scrollEl) updateScrollEdges(scrollEl);
    });

    return () => cancelAnimationFrame(frame);
  });
</script>

<div class={['relative flex min-h-0 min-w-0 flex-1 flex-col', className]}>
  <div
    bind:this={scrollEl}
    {@attach trackScrollEdges}
    class={['flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto', scrollClass]}
    {...rest}
  >
    {@render children()}
  </div>
  {#if top}
    <div
      aria-hidden="true"
      class={[
        'pointer-events-none absolute inset-x-0 top-0 bg-gradient-to-b from-background to-transparent transition-opacity',
        fadeHeight,
        !scrolledFromTop && 'opacity-0'
      ]}
    ></div>
  {/if}
  {#if bottom}
    <div
      aria-hidden="true"
      class={[
        'pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-background to-transparent transition-opacity',
        fadeHeight,
        !scrolledFromBottom && 'opacity-0'
      ]}
    ></div>
  {/if}
</div>
