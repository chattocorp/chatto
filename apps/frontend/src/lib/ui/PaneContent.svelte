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
    /** Allow `max-w-6xl` for browsing grids, such as the room directory masonry. */
    wide?: boolean;
    scrollContainer?: HTMLDivElement;
  } = $props();
</script>

<ScrollFader top bottom bind:scrollEl={scrollContainer}>
  <!-- A zero-length basis keeps tall children bounded inside the min-height content wrapper. -->
  <div
    data-page-reveal
    class={[
      'w-full p-6',
      wide ? 'max-w-6xl' : 'max-w-5xl',
      fillHeight && 'flex min-h-0 flex-1 basis-0 flex-col'
    ]}
  >
    {@render children()}
  </div>
</ScrollFader>
