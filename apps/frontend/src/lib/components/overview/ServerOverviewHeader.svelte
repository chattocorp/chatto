<!--
@component

The identity band at the top of the server Overview: the banner, logo, name,
and description of the current server. A server without a banner shows the
gradient that its name selects, so the band keeps its shape.
-->
<script lang="ts">
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import { getGradientForName } from '$lib/utils/gradients';

  let {
    name,
    logoUrl = null,
    bannerUrl = null,
    description = null
  }: {
    name: string;
    logoUrl?: string | null;
    bannerUrl?: string | null;
    description?: string | null;
  } = $props();

  let failedBannerUrl = $state<string | null>(null);
  const showBanner = $derived(!!bannerUrl && failedBannerUrl !== bannerUrl);
</script>

<header class="flex flex-col" data-testid="server-overview-header">
  <div
    class="h-28 overflow-hidden rounded-xl sm:h-40"
    style:background={showBanner ? undefined : getGradientForName(name)}
  >
    {#if showBanner}
      <img
        src={bannerUrl}
        alt=""
        class="h-full w-full object-cover"
        onerror={() => (failedBannerUrl = bannerUrl)}
      />
    {/if}
  </div>
  <div class="flex min-w-0 items-start gap-4 px-4 sm:px-6">
    <!-- The logo overlaps the banner edge; the background padding separates it from the banner. -->
    <div class="-mt-10 size-20 shrink-0 rounded-2xl bg-background p-1" aria-hidden="true">
      <ServerLogo server={{ name, logoUrl }} fill />
    </div>
    <div class="min-w-0 flex-1 pt-3">
      <h2 class="text-2xl font-bold wrap-anywhere text-text-top"><bdi>{name}</bdi></h2>
      {#if description}
        <p class="mt-1 text-muted" dir="auto">{description}</p>
      {/if}
    </div>
  </div>
</header>
