<script lang="ts">
  import { tick, onMount, type Snippet } from 'svelte';
  import type { VideoProcessingStatus } from '@chatto/client/timeline/messageAttachments';
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import VideoProcessingPlaceholder from './VideoProcessingPlaceholder.svelte';
  import {
    configureBundledHLSProvider,
    recoverFatalHLS,
    shouldAbortHLSRecovery
  } from '$lib/media/hls';
  import { m } from '$lib/i18n/messages';
  import { retainPlayback } from '$lib/media/playbackRetention';
  import {
    positiveVideoDimension as positiveDimension,
    videoFrameStyle
  } from '$lib/media/videoFrame';
  import { LoadingFog, PreviewToolbar } from '$lib/ui';

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
    fallbackUrl = null,
    fallbackContentType = null,
    width = null,
    height = null,
    reasonCode = null,
    filename,
    describedBy,
    autoLoop = false,
    viewer = false,
    onPlaybackChange,
    onMediaError,
    onPosterError,
    actions,
    onOpen
  }: {
    status: VideoProcessingStatus;
    variants?: Variant[];
    thumbnailUrl?: string | null;
    hlsUrl?: string | null;
    fallbackUrl?: string | null;
    fallbackContentType?: string | null;
    width?: number | null;
    height?: number | null;
    reasonCode?: string | null;
    filename: string;
    describedBy?: string;
    autoLoop?: boolean;
    /** Fit the player to the shared attachment viewer rather than a timeline thumbnail. */
    viewer?: boolean;
    /**
     * Report playback until pause, end, failure, source clearing, or player removal.
     * Picture-in-picture keeps playback active until it ends. GIF loops are excluded.
     */
    onPlaybackChange?: (active: boolean) => void;
    onMediaError?: () => void | Promise<string | null>;
    onPosterError?: () => void;
    /** Permission-gated management actions below the player. */
    actions?: Snippet;
    /** Open a converted GIF in the attachment viewer from its preview canvas. */
    onOpen?: () => void;
  } = $props();

  // Existing processed videos can carry stale encoded dimensions. Once the
  // browser loads the media, prefer its intrinsic display size for the frame.
  let measuredMedia = $state<{ src: string; width: number; height: number } | null>(null);
  let hlsRetryUrl = $state<string | null>(null);
  let failedHlsUrl = $state<string | null>(null);

  // Pick the best variant (highest quality available)
  const selectedVariant = $derived(
    variants.length > 0
      ? variants.reduce((best, v) => (v.height > best.height ? v : best), variants[0])
      : null
  );

  // Asset URLs have no file extension. Give Vidstack the media type explicitly.
  const fallbackSource = $derived(
    selectedVariant
      ? ({ src: selectedVariant.url, type: 'video/mp4' } as const)
      : fallbackUrl
        ? { src: fallbackUrl, type: fallbackContentType ?? 'video/mp4' }
        : undefined
  );

  const playbackSource = $derived.by(() => {
    const effectiveHlsUrl = hlsRetryUrl ?? hlsUrl;
    if (!autoLoop && effectiveHlsUrl && effectiveHlsUrl !== failedHlsUrl) {
      return { src: effectiveHlsUrl, type: 'application/vnd.apple.mpegurl' as const };
    }
    return fallbackSource;
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

  const frameStyle = $derived(
    viewer
      ? 'width: 100%; height: 100%; max-width: 100%; max-height: 100%;'
      : videoFrameStyle(sourceDimensions.width, sourceDimensions.height, autoLoop)
  );

  const failureMessage = $derived.by(() => {
    switch (reasonCode) {
      case 'original_missing':
        return m('media.video_original_missing');
      case 'processing_failed':
        return m('media.video_processing_failed_retry');
      default:
        return null;
    }
  });

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
      // Viewer media lives outside the virtual list and can use native fullscreen.
      if (viewer) return;
      e.preventDefault();
      if (!playbackSource) return;

      const video = node.querySelector('video');
      if (video) video.pause();

      const fullscreenFallbackSource =
        playbackSource.type === 'application/vnd.apple.mpegurl' ? (fallbackSource ?? null) : null;
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
        fullscreenFallbackSource,
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
    let hlsRecoveryInProgress = false;

    const handleProviderChange = (event: Event) => {
      configureBundledHLSProvider((event as CustomEvent).detail);
    };
    const handleHLSError = (event: Event) => {
      const detail = (
        event as CustomEvent<{
          fatal?: boolean;
          type?: string;
          details?: string;
        }>
      ).detail;
      const bufferAppendFailed =
        detail?.type === 'mediaError' && detail.details === 'bufferAppendError';
      if (
        !shouldAbortHLSRecovery(detail ?? {}) ||
        hlsRecoveryInProgress ||
        playbackSource?.type !== 'application/vnd.apple.mpegurl'
      ) {
        return;
      }

      hlsRecoveryInProgress = true;
      const rejectedUrl = playbackSource.src;

      // Vidstack otherwise invokes hls.js recoverMediaError() for every fatal
      // media error without a recovery budget. Stop the bad session first so a
      // malformed segment cannot create an endless request loop.
      const provider = (
        node as HTMLElement & { provider?: { instance?: { destroy?: () => void } } }
      ).provider;
      recoverFatalHLS({
        instance: provider?.instance,
        rejectedUrl,
        // A fresh access ticket cannot repair bytes rejected by SourceBuffer.
        refreshUrl: bufferAppendFailed ? undefined : onMediaError,
        retry: (url) => {
          hlsRetryUrl = url;
        },
        fallback: () => {
          failedHlsUrl = rejectedUrl;
        }
      }).finally(() => {
        hlsRecoveryInProgress = false;
      });
    };
    node.addEventListener('provider-change', handleProviderChange);
    node.addEventListener('hls-error', handleHLSError);
    const cleanupFullscreen = interceptFullscreenRequest(node);
    const cleanupVideoObserver = observePlayerVideo(node);

    return () => {
      cleanupFullscreen();
      cleanupVideoObserver();
      node.removeEventListener('provider-change', handleProviderChange);
      node.removeEventListener('hls-error', handleHLSError);
    };
  }
</script>

{#if (status === 'COMPLETED' && playbackSource) || status === 'PENDING' || status === 'PROCESSING'}
  <!-- Keep the same frame while loading, processing, and player registration change. -->
  <div
    class={[
      'my-1.5 preview-frame',
      viewer ? 'flex min-h-0 flex-col' : 'preview-media-frame',
      actions && 'min-w-14'
    ]}
    style={frameStyle}
  >
    {#if status === 'COMPLETED' && selectedVariant && autoLoop}
      {#snippet loopPreview()}
        <!-- Converted GIFs use native video for reliable autoplay and looping. -->
        <video
          autoplay
          loop
          muted
          playsinline
          data-autoloop
          onerror={handlePlayerError}
          onloadedmetadata={handleVideoMetadata}
          style:max-width={autoLoop && actions ? 'var(--preview-media-width)' : undefined}
          class="mx-auto block h-full w-full preview-content object-contain"
          aria-describedby={describedBy}
        >
          <source src={selectedVariant.url} type="video/mp4" onerror={onMediaError} />
        </video>
      {/snippet}
      {#if onOpen}
        <button
          type="button"
          class="mx-auto block h-full w-full cursor-pointer preview-content"
          style:max-width={actions ? 'var(--preview-media-width)' : undefined}
          aria-label={m('room.attachment.view_label', { filename })}
          aria-describedby={describedBy}
          onclick={onOpen}
        >
          {@render loopPreview()}
        </button>
      {:else}
        {@render loopPreview()}
      {/if}
    {:else if status === 'COMPLETED' && elementsReady}
      <!-- Read the prop only on use: a rerun attachment would lose its playback state. -->
      <media-player
        {@attach attachMediaPlayer}
        {@attach retainPlayback((active) => onPlaybackChange?.(active))}
        src={playbackSource}
        stream-type="on-demand"
        playsinline
        onerror={handlePlayerError}
        class={['block h-full w-full preview-content', viewer ? 'min-h-0' : 'min-h-32']}
        aria-describedby={describedBy}
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
        <!-- Vidstack's compact layout omits PiP; retain it beside the top controls. -->
        <media-pip-button
          class="compact-pip vds-button"
          aria-label={m('voice.picture_in_picture')}
          title={m('voice.picture_in_picture')}
        >
          <span class="icon-[mdi--picture-in-picture-bottom-right] text-xl" aria-hidden="true"
          ></span>
        </media-pip-button>
      </media-player>
    {:else if status === 'COMPLETED'}
      <LoadingFog class={autoLoop ? 'h-full w-full' : 'h-full min-h-32 w-full'} />
    {:else}
      <VideoProcessingPlaceholder
        label={status === 'PENDING' ? m('media.video_queued') : m('media.video_processing')}
      />
    {/if}
    {#if actions}<PreviewToolbar>{@render actions()}</PreviewToolbar>{/if}
  </div>
{:else if status === 'FAILED'}
  <div class="my-1.5 preview-frame" style={frameStyle}>
    <div class="flex items-center gap-3 px-4 py-3">
      <span aria-hidden="true" class="iconify icon-[uil--exclamation-triangle] text-lg text-danger"
      ></span>
      <div class="text-sm text-muted">
        {m('media.video_processing_failed')}
        {#if failureMessage}
          <span class="block text-xs text-muted/70">{failureMessage}</span>
        {/if}
      </div>
    </div>
    {#if actions}<PreviewToolbar>{@render actions()}</PreviewToolbar>{/if}
  </div>
{:else}
  <div class="my-1.5 preview-frame" style={frameStyle}>
    <div class="flex items-center gap-2 px-3 py-2">
      <span aria-hidden="true" class="iconify icon-[uil--video] text-lg text-muted"></span>
      <span class="text-sm">{filename}</span>
    </div>
    {#if actions}<PreviewToolbar>{@render actions()}</PreviewToolbar>{/if}
  </div>
{/if}

<style>
  /* Vidstack creates its layout and state attributes outside Svelte markup. */
  .compact-pip {
    display: none;
  }

  :global(
      media-player[data-can-pip][data-controls][data-started]:has(
          media-video-layout[data-size='sm']
        )
    )
    .compact-pip {
    display: flex;
    position: absolute;
    /* Match Vidstack's compact row: 2px padding, 36px buttons, 2.5px gaps. */
    --media-button-size: var(--video-sm-button-size, 36px);
    top: 2px;
    inset-inline-end: calc(var(--video-sm-button-size, 36px) + 7px);
    z-index: 10;
  }

  /* Hide menus from Vidstack's default layout — not useful for embedded chat videos. */
  :global(media-player .vds-settings-menu),
  :global(media-player .vds-chapters-menu) {
    display: none !important;
  }

  /* Viewer players fill their viewport. Inline geometry belongs to the shared
   * preview-media-frame rule, which also sizes the loading placeholder. */
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
