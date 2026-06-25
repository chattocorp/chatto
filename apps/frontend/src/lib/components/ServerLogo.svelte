<script lang="ts">
  import { getGradientForName } from '$lib/utils/gradients';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  /**
   * Minimal data needed for logo display.
   */
  interface ServerForLogo {
    name: string;
    logoUrl?: string | null;
  }

  let {
    server
  }: {
    server: ServerForLogo;
  } = $props();

  const gradientStyle = $derived(server.logoUrl ? undefined : getGradientForName(server.name));
  const initial = $derived(server.name[0]?.toUpperCase() ?? '?');
</script>

<!--
	ServerLogo: Shared component for server icon rendering.
	Shows logo image if available, otherwise gradient background + initial.
	Used by ServerIcon for the server gutter icon.
-->
<div
  class="shimmer-hover flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-xl text-3xl font-black transition-all duration-100"
  style:background={gradientStyle}
>
  {#if server.logoUrl}
    <SkeletonImg src={server.logoUrl} alt={server.name} class="h-full w-full object-cover" />
  {:else}
    <span class="text-white drop-shadow-sm">{initial}</span>
  {/if}
</div>
