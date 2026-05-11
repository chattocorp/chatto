<script lang="ts">
  import { getGradientForName } from '$lib/utils/gradients';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  /**
   * Minimal data needed for logo display.
   */
  interface SpaceForLogo {
    name: string;
    logoUrl?: string | null;
  }

  let {
    space
  }: {
    space: SpaceForLogo;
  } = $props();

  const gradientStyle = $derived(space.logoUrl ? undefined : getGradientForName(space.name));
  const initial = $derived(space.name[0]?.toUpperCase() ?? '?');
</script>

<!--
	SpaceLogo: Shared component for space icon rendering.
	Shows logo image if available, otherwise gradient background + initial.
	Used by SpaceIcon for the sidebar instance icon.
-->
<div
  class="shimmer-hover flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-xl text-3xl font-black transition-all duration-100"
  style:background={gradientStyle}
>
  {#if space.logoUrl}
    <SkeletonImg src={space.logoUrl} alt={space.name} class="h-full w-full object-cover" />
  {:else}
    <span class="text-white drop-shadow-sm">{initial}</span>
  {/if}
</div>
