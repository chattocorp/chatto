<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ScrollFader } from '$lib/ui';

  let {
    children,
    fillHeight = false,
    scrollContainer = $bindable<HTMLDivElement | undefined>()
  }: {
    children: Snippet;
    /** Let a single primary child consume the available page height. */
    fillHeight?: boolean;
    scrollContainer?: HTMLDivElement;
  } = $props();
</script>

<ScrollFader top bottom bind:scrollEl={scrollContainer}>
  <div class={['w-full max-w-5xl p-6', fillHeight && 'flex h-full min-h-0 flex-col']}>
    {@render children()}
  </div>
</ScrollFader>
