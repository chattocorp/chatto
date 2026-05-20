<!--
@component

A single-edge fade overlay for a scroll container. Place inside a
`position: relative` ancestor; the fade absolutely positions itself at
the corresponding edge of that ancestor.

Pass `target` (a scroll container element or getter) to make the fade
auto-hide with an opacity transition when the target is scrolled to the
matching edge.

For a complete top-and-bottom setup wrapped around a scrollable region,
prefer the higher-level [`ScrollFader`](./ScrollFader.svelte) component.
-->
<script lang="ts">
  type Edge = 'top' | 'bottom';

  type Props = {
    /** Scroll container to watch. Fade hides when scrolled to the matching edge. */
    target?: HTMLElement | (() => HTMLElement | null | undefined) | null;
    /** Which scroll edge to react to. Default `bottom`. */
    edge?: Edge;
    /** Tailwind class for the fade height. Default `h-8`. */
    height?: string;
  };

  let { target = null, edge = 'bottom', height = 'h-8' }: Props = $props();

  let visible = $state(false);

  $effect(() => {
    const el = typeof target === 'function' ? target() : target;
    if (!el) {
      visible = false;
      return;
    }
    const update = () => {
      visible =
        edge === 'top'
          ? el.scrollTop > 1
          : el.scrollHeight - el.scrollTop - el.clientHeight > 1;
    };
    update();
    el.addEventListener('scroll', update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', update);
      ro.disconnect();
    };
  });
</script>

<div
  aria-hidden="true"
  class={[
    'pointer-events-none absolute inset-x-0 from-background to-transparent transition-opacity',
    height,
    edge === 'top' ? 'top-0 bg-gradient-to-b' : 'bottom-0 bg-gradient-to-t',
    !visible && 'opacity-0'
  ]}
></div>
