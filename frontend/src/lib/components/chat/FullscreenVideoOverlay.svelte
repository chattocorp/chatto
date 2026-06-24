<!--
	Fullscreen video overlay — renders outside the virtua-virtualized message list.

	The inline VideoPlayer lives inside virtua, which recycles DOM nodes. If we
	fullscreened that element, virtua would unmount it and the browser would
	immediately exit fullscreen (per WHATWG spec). Instead, we render a separate
	Vidstack player in this overlay and request native fullscreen on the overlay
	container — which is outside virtua and safe from recycling.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import * as m from '$lib/i18n/messages';

  import 'vidstack/player/styles/default/theme.css';
  import 'vidstack/player/styles/default/layouts/video.css';

  let elementsReady = $state(false);

  onMount(async () => {
    await Promise.all([
      import('vidstack/player'),
      import('vidstack/player/layouts'),
      import('vidstack/player/ui')
    ]);
    elementsReady = true;
  });

  let playerEl = $state<HTMLElement | null>(null);

  // Seek to captured playback position once the player can play
  $effect(() => {
    if (!playerEl || !fullscreenVideo.isOpen) return;

    function handleCanPlay() {
      if (fullscreenVideo.startTime > 0) {
        const video = playerEl?.querySelector('video');
        if (video) video.currentTime = fullscreenVideo.startTime;
      }
    }

    function blockFullscreen(e: Event) {
      e.preventDefault();
    }

    playerEl.addEventListener('can-play', handleCanPlay, { once: true });
    // Use capture phase so we intercept before Vidstack's internal handler.
    playerEl.addEventListener('media-enter-fullscreen-request', blockFullscreen, true);
    return () => {
      playerEl?.removeEventListener('can-play', handleCanPlay);
      playerEl?.removeEventListener('media-enter-fullscreen-request', blockFullscreen, true);
    };
  });

  function close() {
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    }
    fullscreenVideo.close();
  }

  // When user exits native fullscreen (Escape key or browser controls), close the overlay
  function handleFullscreenChange() {
    if (!document.fullscreenElement && fullscreenVideo.isOpen) {
      fullscreenVideo.close();
    }
  }
</script>

{#if fullscreenVideo.isOpen && fullscreenVideo.src && elementsReady}
  <div
    class="fullscreen-overlay fixed inset-0 z-[9999] flex items-center justify-center bg-black"
    role="dialog"
    aria-modal="true"
    aria-label={m['media.fullscreen_video']()}
    tabindex="-1"
    onfullscreenchange={handleFullscreenChange}
  >
    <button
      class="absolute top-4 right-4 z-10 flex h-10 w-10 cursor-pointer items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/20"
      onclick={close}
      aria-label={m['media.close_fullscreen_video']()}
    >
      <span class="iconify text-2xl uil--times"></span>
    </button>

    <media-player
      bind:this={playerEl}
      src={{ src: fullscreenVideo.src, type: 'video/mp4' }}
      autoplay
      playsinline
      class="h-full w-full"
    >
      <media-provider>
        {#if fullscreenVideo.poster}
          <media-poster class="vds-poster" src={fullscreenVideo.poster} alt={m['media.video_alt']()}
          ></media-poster>
        {/if}
      </media-provider>
      <media-video-layout></media-video-layout>
    </media-player>
  </div>
{/if}

<style>
  :global(.fullscreen-overlay media-player .vds-settings-menu),
  :global(.fullscreen-overlay media-player .vds-chapters-menu),
  :global(.fullscreen-overlay media-player .vds-fullscreen-button) {
    display: none !important;
  }
</style>
