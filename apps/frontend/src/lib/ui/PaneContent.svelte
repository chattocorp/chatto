<script lang="ts">
  import type { Snippet } from 'svelte';
  import ScrollFader from './ScrollFader.svelte';

  let {
    children,
    fillHeight = false,
    wide = false,
    scrollContainer = $bindable<HTMLDivElement | undefined>()
  }: {
    children: Snippet;
    /** Let a single primary child consume the available page height. */
    fillHeight?: boolean;
    /** Allow `max-w-pane-wide` for browsing grids, such as the room directory masonry. */
    wide?: boolean;
    scrollContainer?: HTMLDivElement;
  } = $props();
</script>

<ScrollFader top bottom bind:scrollEl={scrollContainer}>
  <!-- A zero-length basis keeps tall children bounded inside the min-height content wrapper. -->
  <div
    data-page-reveal
    data-pane-content={wide ? 'wide' : undefined}
    class={[
      'w-full p-6',
      wide ? 'max-w-pane-wide' : 'max-w-pane',
      fillHeight && 'flex min-h-0 flex-1 basis-0 flex-col'
    ]}
  >
    {@render children()}
  </div>
</ScrollFader>
