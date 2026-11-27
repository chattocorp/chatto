<!--
@component

The identity band at the top of the server Overview: the banner, logo, name,
and description of the current server.

Banners are authored at 1200×630, which is much taller than the band in a wide
pane. So that the band never crops or enlarges the artwork, the banner shows
uncropped in the center, and a blurred copy fills the rest of the band. A
server without a banner shows the gradient that its name selects, so the band
keeps its shape.
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

<header class="flex flex-col gap-5" data-testid="server-overview-header">
  <div
    class="relative isolate flex h-40 items-center justify-center overflow-hidden rounded-xl sm:h-56"
    style:background={showBanner ? undefined : getGradientForName(name)}
  >
    {#if showBanner}
      <div class="relative aspect-[1200/630] h-full max-w-full">
        <!--
          Each side of the band continues the banner's outer edge: a mirrored
          copy is stretched far outward, so only the edge columns stay
          visible. Each copy starts at the banner's center, so the two copies
          also fill the area behind the banner and leave no gap for the blur
          to pull in. The parent blurs after the stretch and is taller than
          the band, so the blur's transparent fringe falls outside the band.
          The sides follow the image, not the reading direction.
        -->
        <div class="absolute inset-x-0 -inset-y-8 -z-10 blur-xl">
          {#each ['left-1/2 origin-left', 'right-1/2 origin-right'] as side (side)}
            <img
              src={bannerUrl}
              alt=""
              class={[
                'absolute inset-y-0 h-full w-full max-w-none [scale:-24_1] object-fill',
                side
              ]}
            />
          {/each}
        </div>
        <img
          src={bannerUrl}
          alt=""
          class="size-full object-cover"
          onerror={() => (failedBannerUrl = bannerUrl)}
        />
      </div>
    {/if}
  </div>
  <div class="flex min-w-0 items-center gap-4 px-1">
    <div class="size-16 shrink-0" aria-hidden="true">
      <ServerLogo server={{ name, logoUrl }} fill />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-2xl font-bold wrap-anywhere text-text-top"><bdi>{name}</bdi></h2>
      {#if description}
        <p class="mt-1 text-muted" dir="auto">{description}</p>
      {/if}
    </div>
  </div>
</header>
