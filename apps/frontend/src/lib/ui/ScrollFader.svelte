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

  let scrolledFromTop = $state(false);
  let scrolledFromBottom = $state(false);

  function updateScrollEdges(el: HTMLElement) {
    const maxScrollTop = Math.max(0, el.scrollHeight - el.clientHeight);
    const scrollTop = Math.min(Math.max(el.scrollTop, 0), maxScrollTop);
    const canScroll = maxScrollTop > 1;

    scrolledFromTop = canScroll && scrollTop > 1;
    scrolledFromBottom = canScroll && maxScrollTop - scrollTop > 1;
  }

  function trackScrollEdges(el: HTMLElement) {
    const update = () => {
      updateScrollEdges(el);
    };

    update();
    el.addEventListener('scroll', update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(el);
    for (const child of el.children) {
      if (child instanceof HTMLElement) ro.observe(child);
    }
    const mo = new MutationObserver(() => {
      ro.disconnect();
      ro.observe(el);
      for (const child of el.children) {
        if (child instanceof HTMLElement) ro.observe(child);
      }
      update();
    });
    mo.observe(el, { childList: true });
    return () => {
      el.removeEventListener('scroll', update);
      mo.disconnect();
      ro.disconnect();
    };
  }

  export function refresh() {
    if (!scrollEl) return;

    requestAnimationFrame(() => {
      if (scrollEl) updateScrollEdges(scrollEl);
    });
  }
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
