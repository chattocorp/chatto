<script lang="ts">
  import { tick, onMount } from 'svelte';
  import type { VideoProcessingStatus } from '$lib/render/types';
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import { configureBundledHLSProvider } from '$lib/media/hls';
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
    hlsUrl = null,
    width = null,
    height = null,
    reasonCode = null,
    filename,
    autoLoop = false,
    onMediaError,
    onPosterError
  }: {
    status: VideoProcessingStatus;
    variants?: Variant[];
    thumbnailUrl?: string | null;
    hlsUrl?: string | null;
    width?: number | null;
    height?: number | null;
    reasonCode?: string | null;
    filename: string;
    autoLoop?: boolean;
    onMediaError?: () => void | Promise<string | null>;
    onPosterError?: () => void;
  } = $props();

  const MAX_WIDTH = 480;
  const MAX_HEIGHT = 320;
  const MIN_PLAYER_ASPECT_RATIO = 9 / 16;
  const MAX_PLAYER_ASPECT_RATIO = 16 / 9;

  // Existing processed videos can carry stale encoded dimensions. Once the
  // browser loads the media, prefer its intrinsic display size for the frame.
  let measuredMedia = $state<{ src: string; width: number; height: number } | null>(null);

  // Pick the best variant (highest quality available)
  const selectedVariant = $derived(
    variants.length > 0
      ? variants.reduce((best, v) => (v.height > best.height ? v : best), variants[0])
      : null
  );

  const playbackSource = $derived.by(() => {
    if (!autoLoop && hlsUrl) {
      return { src: hlsUrl, type: 'application/vnd.apple.mpegurl' as const };
    }
    return selectedVariant ? { src: selectedVariant.url, type: 'video/mp4' as const } : undefined;
  });

  const sourceDimensions = $derived.by(() => {
    if (measuredMedia && measuredMedia.src === playbackSource?.src) {
      return measuredMedia;
    }
    return {
      width: positiveDimension(width) ?? positiveDimension(selectedVariant?.width) ?? 480,
      height: positiveDimension(height) ?? positiveDimension(selectedVariant?.height) ?? 270
    };
  });

  const displaySize = $derived.by(() => {
    const w = sourceDimensions.width;
    const h = sourceDimensions.height;
    const mediaAspectRatio = w / h;

    // Vidstack's controls need a usable canvas even when the media itself is
    // extremely tall or wide. Clamp only the player canvas; object-fit keeps
    // the complete video at its true aspect ratio inside it.
    const canvasWidth =
      mediaAspectRatio < MIN_PLAYER_ASPECT_RATIO ? h * MIN_PLAYER_ASPECT_RATIO : w;
    const canvasHeight =
      mediaAspectRatio > MAX_PLAYER_ASPECT_RATIO ? w / MAX_PLAYER_ASPECT_RATIO : h;
    const scale = Math.min(MAX_WIDTH / canvasWidth, MAX_HEIGHT / canvasHeight, 1);
    return {
      width: Math.max(1, Math.round(canvasWidth * scale)),
      height: Math.max(1, Math.round(canvasHeight * scale))
    };
  });

  const frameStyle = $derived(
    `width: ${displaySize.width}px; max-width: 100%; aspect-ratio: ${displaySize.width} / ${displaySize.height};`
  );

  // Vidstack auto-detects media type from URL extensions, but our stable asset
  // URLs have no extension (/assets/files/...). We must provide an
  // explicit type so Vidstack recognizes it as video/mp4.
  const videoSrc = $derived(playbackSource);

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
    if (!playbackSource) return;
    const videoWidth = positiveDimension(video.videoWidth);
    const videoHeight = positiveDimension(video.videoHeight);
    if (!videoWidth || !videoHeight) return;
    measuredMedia = {
      src: playbackSource.src,
      width: videoWidth,
      height: videoHeight
    };
  }

  function handleVideoMetadata(event: Event) {
    if (event.currentTarget instanceof HTMLVideoElement) {
      syncVideoDimensions(event.currentTarget);
    }
  }

  function handlePlayerError() {
    onMediaError?.();
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
      if (!playbackSource) return;

      const video = node.querySelector('video');
      if (video) video.pause();

      const fallbackSource =
        playbackSource.type === 'application/vnd.apple.mpegurl' && selectedVariant
          ? ({ src: selectedVariant.url, type: 'video/mp4' } as const)
          : null;
      const refreshSource =
        playbackSource.type === 'application/vnd.apple.mpegurl' && onMediaError
          ? async () => {
              const refreshedURL = await onMediaError();
              return typeof refreshedURL === 'string'
                ? ({ src: refreshedURL, type: 'application/vnd.apple.mpegurl' } as const)
                : null;
            }
          : null;
      fullscreenVideo.open(
        playbackSource,
        thumbnailUrl ?? null,
        video?.currentTime ?? 0,
        fallbackSource,
        refreshSource
      );

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
    const handleProviderChange = (event: Event) => {
      configureBundledHLSProvider((event as CustomEvent).detail);
    };
    node.addEventListener('provider-change', handleProviderChange);
    const cleanupFullscreen = interceptFullscreenRequest(node);
    const cleanupVideoObserver = observePlayerVideo(node);

    return () => {
      cleanupFullscreen();
      cleanupVideoObserver();
      node.removeEventListener('provider-change', handleProviderChange);
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
      onerror={handlePlayerError}
      onloadedmetadata={handleVideoMetadata}
      class="block h-full w-full object-contain"
    >
      <source src={selectedVariant.url} type="video/mp4" onerror={onMediaError} />
    </video>
  </div>
{:else if status === 'COMPLETED' && (hlsUrl || selectedVariant) && elementsReady}
  <div class="embed-frame" style={frameStyle}>
    <media-player
      {@attach attachMediaPlayer}
      src={videoSrc}
      playsinline
      onerror={handlePlayerError}
      class="block h-full w-full"
    >
      <media-provider>
        {#if thumbnailUrl}
          <media-poster
            class="vds-poster"
            src={thumbnailUrl}
            alt={filename}
            onerror={onPosterError ?? onMediaError}
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
    <span class="iconify text-lg text-danger uil--exclamation-triangle"></span>
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

  /* Vidstack defaults every video player to 16:9. Inherit the attachment's
   * measured ratio and fill that frame while preserving the whole image. */
  :global(media-player) {
    aspect-ratio: inherit;
    height: 100%;
    width: 100%;
  }

  :global(media-player media-provider),
  :global(media-player [data-media-provider]),
  :global(media-player video),
  :global(media-player .vds-poster),
  :global(media-player .vds-poster img) {
    height: 100%;
    width: 100%;
  }

  :global(media-player video),
  :global(media-player .vds-poster),
  :global(media-player .vds-poster img) {
    object-fit: contain;
  }
</style>
