<script lang="ts">
  import { tick, onMount } from 'svelte';
  import type { VideoProcessingStatus } from '$lib/render/types';
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import * as m from '$lib/i18n/messages';

  import 'vidstack/player/styles/default/theme.css';
  import 'vidstack/player/styles/default/layouts/video.css';

  // Vidstack ships empty server stubs under the "default" export condition;
  // static imports in SvelteKit resolve those stubs during SSR and never
  // re-run on the client. We must dynamically import on mount and wait for
  // registration to complete before rendering the custom elements.
  let elementsReady = $state(false);

  onMount(async () => {
    await Promise.all([
      import('vidstack/player'),
      import('vidstack/player/layouts'),
      import('vidstack/player/ui')
    ]);
    elementsReady = true;
  });

  type Variant = {
    url: string;
    quality: string;
    width: number;
    height: number;
    size: number;
  };

  let {
    status,
    variants = [],
    thumbnailUrl = null,
    width = null,
    height = null,
    reasonCode = null,
    filename,
    autoLoop = false,
    onMediaError
  }: {
    status: VideoProcessingStatus;
    variants?: Variant[];
    thumbnailUrl?: string | null;
    width?: number | null;
    height?: number | null;
    reasonCode?: string | null;
    filename: string;
    autoLoop?: boolean;
    onMediaError?: () => void;
  } = $props();

  const MAX_WIDTH = 480;
  const MAX_HEIGHT = 320;
  const WIDESCREEN_RATIO = 16 / 9;
  const NEAR_SQUARE_LANDSCAPE_MAX_RATIO = 1.5;

  // Existing processed videos can carry stale encoded dimensions. Once the
  // browser loads the media, prefer its intrinsic display size for the frame.
  let measuredMedia = $state<{ src: string; width: number; height: number } | null>(null);

  // Pick the best variant (highest quality available)
  const selectedVariant = $derived(
    variants.length > 0
      ? variants.reduce((best, v) => (v.height > best.height ? v : best), variants[0])
      : null
  );

  const sourceDimensions = $derived.by(() => {
    if (measuredMedia && measuredMedia.src === selectedVariant?.url) {
      return measuredMedia;
    }
    return {
      width: positiveDimension(width) ?? positiveDimension(selectedVariant?.width) ?? 480,
      height: positiveDimension(height) ?? positiveDimension(selectedVariant?.height) ?? 270
    };
  });

  const frameDimensions = $derived.by(() => {
    const w = sourceDimensions.width;
    const h = sourceDimensions.height;
    const ratio = w / h;

    if (
      status === 'COMPLETED' &&
      selectedVariant &&
      !autoLoop &&
      ratio >= 1 &&
      ratio < NEAR_SQUARE_LANDSCAPE_MAX_RATIO
    ) {
      return {
        width: Math.round(h * WIDESCREEN_RATIO),
        height: h
      };
    }

    return { width: w, height: h };
  });

  const displaySize = $derived.by(() => {
    const w = frameDimensions.width;
    const h = frameDimensions.height;
    const scale = Math.min(MAX_WIDTH / w, MAX_HEIGHT / h, 1);
    return {
      width: Math.round(w * scale),
      height: Math.round(h * scale)
    };
  });

  const fitMode = $derived.by(() => {
    if (status !== 'COMPLETED' || !selectedVariant || autoLoop) return 'contain';
    return frameDimensions.width / frameDimensions.height >
      sourceDimensions.width / sourceDimensions.height
      ? 'cover'
      : 'contain';
  });

  const frameStyle = $derived(
    `width: ${displaySize.width}px; max-width: 100%; aspect-ratio: ${displaySize.width} / ${displaySize.height};`
  );

  // Vidstack auto-detects media type from URL extensions, but our stable asset
  // URLs have no extension (/assets/files/...). We must provide an
  // explicit type so Vidstack recognizes it as video/mp4.
  const videoSrc = $derived(
    selectedVariant ? { src: selectedVariant.url, type: 'video/mp4' } : undefined
  );

  const failureMessage = $derived.by(() => {
    switch (reasonCode) {
      case 'original_missing':
        return m['media.video_original_missing']();
      case 'processing_failed':
        return m['media.video_processing_failed_retry']();
      default:
        return null;
    }
  });

  function positiveDimension(value: number | null | undefined): number | null {
    return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null;
  }

  function syncVideoDimensions(video: HTMLVideoElement) {
    if (!selectedVariant) return;
    const videoWidth = positiveDimension(video.videoWidth);
    const videoHeight = positiveDimension(video.videoHeight);
    if (!videoWidth || !videoHeight) return;
    measuredMedia = {
      src: selectedVariant.url,
      width: videoWidth,
      height: videoHeight
    };
  }

  function handleVideoMetadata(event: Event) {
    if (event.currentTarget instanceof HTMLVideoElement) {
      syncVideoDimensions(event.currentTarget);
    }
  }

  function observePlayerVideo(node: HTMLElement) {
    let video: HTMLVideoElement | null = null;
    let removeVideoListener: (() => void) | null = null;

    function bindVideo() {
      const nextVideo = node.querySelector('video');
      if (nextVideo === video) return;

      removeVideoListener?.();
      video = nextVideo;
      removeVideoListener = null;

      if (!video) return;
      const handleMetadata = () => syncVideoDimensions(video!);
      video.addEventListener('loadedmetadata', handleMetadata);
      removeVideoListener = () => video?.removeEventListener('loadedmetadata', handleMetadata);
      syncVideoDimensions(video);
    }

    bindVideo();
    const observer = new MutationObserver(bindVideo);
    observer.observe(node, { childList: true, subtree: true });

    return () => {
      observer.disconnect();
      removeVideoListener?.();
    };
  }

  // Intercept Vidstack's fullscreen request — the <media-player> lives inside
  // virtua's virtualized list, so native fullscreen would cause virtua to
  // unmount the DOM node. Instead, open our CSS overlay outside the list.
  function interceptFullscreenRequest(node: HTMLElement) {
    function handleFullscreenRequest(e: Event) {
      e.preventDefault();
      if (!selectedVariant) return;

      const video = node.querySelector('video');
      if (video) video.pause();

      fullscreenVideo.open(selectedVariant.url, thumbnailUrl ?? null, video?.currentTime ?? 0);

      // Request native fullscreen on the overlay after Svelte renders it.
      // tick() preserves the user activation from this click event.
      tick().then(() => {
        document
          .querySelector('.fullscreen-overlay')
          ?.requestFullscreen()
          .catch(() => {});
      });
    }

    // Use capture phase so we intercept before Vidstack's internal handler.
    node.addEventListener('media-enter-fullscreen-request', handleFullscreenRequest, true);
    return () => {
      node.removeEventListener('media-enter-fullscreen-request', handleFullscreenRequest, true);
    };
  }

  function attachMediaPlayer(node: HTMLElement) {
    const cleanupFullscreen = interceptFullscreenRequest(node);
    const cleanupVideoObserver = observePlayerVideo(node);

    return () => {
      cleanupFullscreen();
      cleanupVideoObserver();
    };
  }
</script>

{#if status === 'COMPLETED' && selectedVariant && autoLoop}
  <!-- Converted GIFs use a native <video> for reliable autoplay + loop behavior. -->
  <div class="embed-frame" style={frameStyle}>
    <video
      autoplay
      loop
      muted
      playsinline
      data-autoloop
      onerror={onMediaError}
      onloadedmetadata={handleVideoMetadata}
      class="block h-full w-full object-contain"
    >
      <source src={selectedVariant.url} type="video/mp4" onerror={onMediaError} />
    </video>
  </div>
{:else if status === 'COMPLETED' && selectedVariant && elementsReady}
  <div class="embed-frame" style={frameStyle}>
    <media-player
      {@attach attachMediaPlayer}
      src={videoSrc}
      playsinline
      onerror={onMediaError}
      data-fit={fitMode}
      class="block h-full w-full"
    >
      <media-provider>
        {#if thumbnailUrl}
          <media-poster class="vds-poster" src={thumbnailUrl} alt={filename} onerror={onMediaError}
          ></media-poster>
        {/if}
      </media-provider>
      <media-video-layout></media-video-layout>
    </media-player>
  </div>
{:else if status === 'PENDING' || status === 'PROCESSING'}
  <div class="embed-frame flex items-center gap-3 px-4 py-3" style={frameStyle}>
    <div class="h-5 w-5 animate-spin rounded-full border-2 border-muted border-t-transparent"></div>
    <div class="text-sm text-muted">
      {status === 'PENDING' ? m['media.video_queued']() : m['media.video_processing']()}
    </div>
  </div>
{:else if status === 'FAILED'}
  <div class="embed-frame flex items-center gap-3 px-4 py-3" style={frameStyle}>
    <span class="iconify text-lg text-red-400 uil--exclamation-triangle"></span>
    <div class="text-sm text-muted">
      {m['media.video_processing_failed']()}
      {#if failureMessage}
        <span class="block text-xs text-muted/70">{failureMessage}</span>
      {/if}
    </div>
  </div>
{:else}
  <div class="embed-frame flex items-center gap-2 px-3 py-2">
    <span class="iconify text-lg text-muted uil--video"></span>
    <span class="text-sm">{filename}</span>
  </div>
{/if}

<style>
  /* Hide menus from Vidstack's default layout — not useful for embedded chat videos. */
  :global(media-player .vds-settings-menu),
  :global(media-player .vds-chapters-menu) {
    display: none !important;
  }

  :global(media-player[data-fit='cover'] media-provider),
  :global(media-player[data-fit='cover'] [data-media-provider]),
  :global(media-player[data-fit='cover'] video),
  :global(media-player[data-fit='cover'] .vds-poster),
  :global(media-player[data-fit='cover'] .vds-poster img) {
    height: 100%;
    width: 100%;
  }

  :global(media-player[data-fit='cover'] video),
  :global(media-player[data-fit='cover'] .vds-poster),
  :global(media-player[data-fit='cover'] .vds-poster img) {
    object-fit: cover;
    object-position: top center;
  }
</style>
