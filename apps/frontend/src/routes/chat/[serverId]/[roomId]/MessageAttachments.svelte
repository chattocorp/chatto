<script lang="ts">
  import { trackScrollEdges, type ScrollEdges } from '$lib/ui/scrollEdges';
  import { type MessageAttachmentView } from '$lib/render/messageAttachments';

  type RawAttachment = MessageAttachmentView;
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { pushState } from '$app/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    assetUrlNeedsRefresh,
    createAssetUrlRetainer,
    earliestAssetUrlRefreshAt,
    mergeRefreshedAttachmentUrls,
    refreshAttachmentUrlsForAssets,
    withAssetUrlRetryParam,
    type ExpiringAssetUrl,
    type RefreshedAttachmentUrls
  } from '$lib/attachments/attachmentUrls';
  import { createAttachmentAPI } from '$lib/api-client/attachments';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { useExpiringAssetUrlRefresh } from '$lib/attachments/useExpiringAssetUrlRefresh.svelte';

  let videoPlayerModule: Promise<typeof import('$lib/components/chat/VideoPlayer.svelte')> | null =
    null;
  let videoPlayerLoadAttempt = $state(0);

  function loadVideoPlayer(_attempt: number) {
    videoPlayerModule ??= import('$lib/components/chat/VideoPlayer.svelte').catch(
      (error: unknown) => {
        videoPlayerModule = null;
        throw error;
      }
    );
    return videoPlayerModule;
  }

  let {
    attachments: rawAttachments,
    serverId,
    roomId,
    eventId,
    canDeleteAttachment = false,
    canEditAttachmentDescription = false
  }: {
    attachments: readonly MessageAttachmentView[];
    serverId: string;
    roomId: string;
    eventId: string;
    canDeleteAttachment?: boolean;
    canEditAttachmentDescription?: boolean;
  } = $props();

  let refreshedAttachmentUrls = $state.raw(new Map<string, RefreshedAttachmentUrls>());
  const assetRetrySalts = new SvelteMap<string, number>();
  let refreshPromise: Promise<Map<string, RefreshedAttachmentUrls>> | null = null;
  const failedAssetRefreshKeys = new SvelteSet<string>();
  const retainAssetUrl = createAssetUrlRetainer();
  let galleryEdges = $state<ScrollEdges>({ start: false, end: false });

  function normalizeAssetUrl(value: ExpiringAssetUrl | null | undefined): ExpiringAssetUrl | null {
    if (!value) return null;
    return {
      ...value,
      url: assetUrlForServer(serverId, value.url) ?? value.url
    };
  }

  function withRetrySalt(
    value: ExpiringAssetUrl | null,
    attachmentID: string,
    role: string
  ): ExpiringAssetUrl | null {
    if (!value) return null;
    const salt = assetRetrySalts.get(`${attachmentID}:${role}`);
    return salt ? { ...value, url: withAssetUrlRetryParam(value.url, salt) } : value;
  }

  function refreshedVariantAssetUrl(
    refreshed: RefreshedAttachmentUrls | undefined,
    quality: string,
    fallback: ExpiringAssetUrl | null | undefined
  ): ExpiringAssetUrl | null | undefined {
    return refreshed ? (refreshed.variantAssetUrls.get(quality) ?? null) : fallback;
  }

  function normalizeAttachment(attachment: RawAttachment) {
    const refreshed = refreshedAttachmentUrls.get(attachment.id);
    const resolveUrl = (
      role: string,
      value: ExpiringAssetUrl | null | undefined,
      retryRole = role
    ) =>
      retainAssetUrl(
        `${attachment.id}:${role}`,
        withRetrySalt(normalizeAssetUrl(value), attachment.id, retryRole),
        refreshed !== undefined || assetRetrySalts.has(`${attachment.id}:${retryRole}`)
      );
    const assetUrl = resolveUrl('asset', refreshed ? refreshed.assetUrl : attachment.assetUrl);
    const thumbnailAssetUrl = resolveUrl(
      'thumbnail',
      refreshed ? refreshed.thumbnailAssetUrl : attachment.thumbnailAssetUrl
    );
    const videoThumbnailAssetUrl = resolveUrl(
      'video-thumbnail',
      refreshed ? refreshed.videoThumbnailAssetUrl : attachment.videoProcessing?.thumbnailAssetUrl,
      'video'
    );
    const hlsMasterPlaylistUrl = resolveUrl(
      'hls',
      refreshed ? refreshed.hlsMasterPlaylistUrl : attachment.videoProcessing?.hlsMasterPlaylistUrl,
      'hls'
    );

    return {
      ...attachment,
      assetUrl,
      url: assetUrl?.url ?? null,
      thumbnailAssetUrl,
      thumbnailUrl: thumbnailAssetUrl?.url ?? null,
      videoProcessing: attachment.videoProcessing
        ? {
            ...attachment.videoProcessing,
            thumbnailAssetUrl: videoThumbnailAssetUrl,
            thumbnailUrl: videoThumbnailAssetUrl?.url ?? null,
            hlsMasterPlaylistUrl,
            hlsUrl: hlsMasterPlaylistUrl?.url ?? null,
            variants: attachment.videoProcessing.variants.flatMap((variant) => {
              const variantAssetUrl = resolveUrl(
                `variant:${variant.quality}`,
                refreshedVariantAssetUrl(refreshed, variant.quality, variant.assetUrl),
                'video'
              );
              if (!variantAssetUrl) return [];
              return {
                ...variant,
                assetUrl: variantAssetUrl,
                url: variantAssetUrl.url
              };
            })
          }
        : null
    };
  }

  function descriptionID(attachment: Attachment): string {
    return `attachment-description-${eventId}-${attachment.id}`;
  }

  type Attachment = ReturnType<typeof normalizeAttachment>;

  const attachments = $derived.by(() =>
    rawAttachments.map((attachment) => normalizeAttachment(attachment))
  );

  const MIN_THUMB_SIZE = 24;
  const EXTREME_ASPECT_RATIO = 3;
  const LANDSCAPE_THUMB_MAX_WIDTH = 480;
  const PORTRAIT_THUMB_MAX_WIDTH = 320;
  const SINGLE_THUMB_MAX_HEIGHT = 200;
  const GALLERY_THUMB_HEIGHT = 180;
  const GALLERY_THUMB_MIN_WIDTH = 72;
  const GALLERY_THUMB_MAX_WIDTH = 320;

  type ThumbDisplay = {
    width: number;
    height: number;
    fit: 'cover' | 'contain';
  };

  function fitThumbWithinBounds(
    w: number,
    h: number,
    maxW: number,
    maxH: number,
    fit: 'cover' | 'contain'
  ) {
    const scale = Math.min(maxW / w, maxH / h, 1);
    return {
      width: Math.max(Math.round(w * scale), MIN_THUMB_SIZE),
      height: Math.max(Math.round(h * scale), MIN_THUMB_SIZE),
      fit
    };
  }

  function thumbDisplay(w: number, h: number) {
    const isLandscape = w > h;
    const aspectRatio = w / h;
    const maxW = isLandscape ? LANDSCAPE_THUMB_MAX_WIDTH : PORTRAIT_THUMB_MAX_WIDTH;

    const fit =
      aspectRatio >= EXTREME_ASPECT_RATIO || aspectRatio <= 1 / EXTREME_ASPECT_RATIO
        ? 'contain'
        : 'cover';
    return fitThumbWithinBounds(w, h, maxW, SINGLE_THUMB_MAX_HEIGHT, fit);
  }

  function galleryThumbDisplay(w: number, h: number): ThumbDisplay {
    const aspectRatio = w / h;
    return {
      width: Math.min(
        Math.max(Math.round(GALLERY_THUMB_HEIGHT * aspectRatio), GALLERY_THUMB_MIN_WIDTH),
        GALLERY_THUMB_MAX_WIDTH
      ),
      height: GALLERY_THUMB_HEIGHT,
      fit:
        aspectRatio >= EXTREME_ASPECT_RATIO || aspectRatio <= 1 / EXTREME_ASPECT_RATIO
          ? 'contain'
          : 'cover'
    };
  }

  function fallbackGalleryThumbDisplay(): ThumbDisplay {
    return {
      width: GALLERY_THUMB_HEIGHT,
      height: GALLERY_THUMB_HEIGHT,
      fit: 'contain'
    };
  }

  function isGalleryImageAttachment(attachment: Attachment): boolean {
    return (
      attachment.contentType.startsWith('image/') &&
      !(attachment.contentType === 'image/gif' && attachment.videoProcessing)
    );
  }

  function imageButtonStyle(display: ThumbDisplay, variant: 'single' | 'gallery'): string {
    if (variant === 'gallery') {
      return `width: ${display.width}px; height: ${display.height}px`;
    }
    return `width: ${display.width}px; max-width: 100%; aspect-ratio: ${display.width} / ${display.height}`;
  }

  function imageAttachmentUrl(attachment: Attachment): string | null {
    return attachment.thumbnailUrl ?? attachment.url;
  }

  const trackGalleryScrollEdges = trackScrollEdges('x', (edges) => {
    galleryEdges = edges;
  });

  const imageAttachments = $derived(attachments.filter(isGalleryImageAttachment));
  const hasImageGallery = $derived(imageAttachments.length > 1);
  const remainingAttachments = $derived(
    hasImageGallery ? attachments.filter((a) => !isGalleryImageAttachment(a)) : attachments
  );

  const serverScope = useServerScope();

  function attachmentAssetUrls(attachment: Attachment) {
    return [
      attachment.assetUrl,
      attachment.thumbnailAssetUrl,
      attachment.videoProcessing?.thumbnailAssetUrl,
      attachment.videoProcessing?.hlsMasterPlaylistUrl,
      ...(attachment.videoProcessing?.variants.map((variant) => variant.assetUrl) ?? [])
    ];
  }

  const nextAssetUrlRefreshAt = $derived.by(() => {
    return earliestAssetUrlRefreshAt(
      attachments.flatMap((attachment) => attachmentAssetUrls(attachment))
    );
  });

  function hasRefreshableStaleUrl() {
    return attachments.some((attachment) =>
      attachmentAssetUrls(attachment).some((assetUrl) => assetUrlNeedsRefresh(assetUrl))
    );
  }

  async function refreshAndApplyUrls(): Promise<Map<string, RefreshedAttachmentUrls>> {
    if (refreshPromise) return refreshPromise;

    refreshPromise = refreshUrlsForMessage()
      .then((freshUrls) => {
        if (freshUrls.size > 0) {
          refreshedAttachmentUrls = mergeRefreshedAttachmentUrls(
            refreshedAttachmentUrls,
            freshUrls
          );
        }
        return freshUrls;
      })
      .finally(() => {
        refreshPromise = null;
      });

    return refreshPromise;
  }

  async function refreshAfterAssetError(
    attachment: Attachment,
    role: string
  ): Promise<string | null> {
    const key = `${attachment.id}:${role}`;
    if (failedAssetRefreshKeys.has(key)) return null;
    if (role !== 'hls') failedAssetRefreshKeys.add(key);
    try {
      const freshUrls = await refreshAndApplyUrls();
      if (role !== 'hls') {
        assetRetrySalts.set(key, Date.now());
        return null;
      }

      // The refresh helper deliberately converts request failures into an
      // empty map. Only consume HLS's one-shot media recovery after this
      // specific request returned a usable replacement ticket; retrying the
      // previous URL with a cache-buster cannot repair an expired ticket.
      const value = freshUrls.get(attachment.id)?.hlsMasterPlaylistUrl;
      if (!value?.url) return null;

      failedAssetRefreshKeys.add(key);
      assetRetrySalts.set(key, Date.now());
      return withRetrySalt(normalizeAssetUrl(value), attachment.id, role)?.url ?? null;
    } catch (error: unknown) {
      console.warn('Failed to refresh attachment URL after load error', error);
      return null;
    }
  }

  useExpiringAssetUrlRefresh({
    getRefreshAt: () => nextAssetUrlRefreshAt,
    hasStaleUrl: hasRefreshableStaleUrl,
    refresh: refreshAndApplyUrls,
    errorMessage: 'Failed to refresh attachment URLs'
  });

  async function refreshUrlsForMessage(): Promise<Map<string, RefreshedAttachmentUrls>> {
    return refreshAttachmentUrlsForAssets(
      currentAttachmentAPI(),
      roomId,
      attachments.map((attachment) => attachment.id)
    );
  }

  function currentAttachmentAPI() {
    return serverScope.connection.getAPI(createAttachmentAPI);
  }

  function openAttachmentModal(attachment: Attachment) {
    // Capture file identities and URLs; media state stays local to the viewer.
    const items =
      attachment.contentType.startsWith('image/') && !attachment.videoProcessing
        ? attachments.filter((a) => a.contentType.startsWith('image/') && !a.videoProcessing)
        : attachments.filter((a) => a.id === attachment.id);
    pushState('', {
      modal: {
        type: 'attachmentViewer',
        serverId,
        roomId,
        eventId,
        items,
        index: items.findIndex((a) => a.id === attachment.id)
      }
    });
  }

  function openDeleteConfirmation(attachment: Attachment, event: Event) {
    // Prevent opening the image modal
    event.stopPropagation();

    pushState('', {
      modal: {
        type: 'deleteAttachment',
        serverId,
        roomId,
        eventId,
        attachmentId: attachment.id
      }
    });
  }

  function openDescriptionEditor(attachment: Attachment, event: Event) {
    event.stopPropagation();
    pushState('', {
      modal: {
        type: 'editAttachmentDescription',
        serverId,
        roomId,
        eventId,
        attachmentId: attachment.id,
        description: attachment.description ?? ''
      }
    });
  }
</script>

{#if attachments.length > 0}
  {#snippet deleteAttachmentButton(attachment: Attachment)}
    {#if canDeleteAttachment}
      <button
        type="button"
        onclick={(event) => openDeleteConfirmation(attachment, event)}
        class="btn-danger-secondary attachment-action-button"
        aria-label={m('room.attachment.delete_label')}
        title={m('room.attachment.delete_label')}
      >
        <span class="iconify icon-[uil--trash-alt] text-sm" aria-hidden="true"></span>
      </button>
    {/if}
  {/snippet}

  {#snippet editDescriptionButton(attachment: Attachment)}
    {#if canEditAttachmentDescription}
      <button
        type="button"
        onclick={(event) => openDescriptionEditor(attachment, event)}
        class="btn-secondary attachment-action-button"
        aria-label={attachment.description
          ? m('room.attachment.edit_description')
          : m('room.attachment.add_description')}
        title={attachment.description
          ? m('room.attachment.edit_description')
          : m('room.attachment.add_description')}
      >
        <span class="iconify icon-[uil--file-edit-alt] text-sm" aria-hidden="true"></span>
      </button>
    {/if}
  {/snippet}

  {#snippet attachmentControls(
    attachment: Attachment,
    showViewer = false,
    layout: 'overlay' | 'row' = 'overlay'
  )}
    {#if canDeleteAttachment || showViewer || canEditAttachmentDescription}
      <div
        class={[
          'z-10 flex gap-1',
          layout === 'row' ? 'max-w-full flex-wrap items-center' : 'shrink-0 flex-col',
          layout === 'overlay' && 'absolute top-3 right-2',
          layout !== 'row' &&
            'transition-opacity feedback-quick compact-input:hover-actions:opacity-0 group-hover/attachment:opacity-100 focus-within:opacity-100'
        ]}
      >
        {@render deleteAttachmentButton(attachment)}
        {#if showViewer}
          {@render viewAttachmentButton(attachment)}
        {/if}
        {@render editDescriptionButton(attachment)}
      </div>
    {/if}
  {/snippet}

  {#snippet imageAttachmentButton(attachment: Attachment, variant: 'single' | 'gallery')}
    {@const display =
      attachment.width && attachment.height
        ? variant === 'gallery'
          ? galleryThumbDisplay(attachment.width, attachment.height)
          : thumbDisplay(attachment.width, attachment.height)
        : variant === 'gallery'
          ? fallbackGalleryThumbDisplay()
          : null}
    <div
      class={[
        'group/attachment relative min-w-0',
        variant === 'gallery' ? 'shrink-0' : 'max-w-full'
      ]}
    >
      <button
        type="button"
        onclick={() => openAttachmentModal(attachment)}
        data-message-image-attachment
        aria-label={m('room.attachment.view_label', { filename: attachment.filename })}
        aria-describedby={attachment.description ? descriptionID(attachment) : undefined}
        data-testid={variant === 'gallery' ? 'message-gallery-image' : undefined}
        style={display ? imageButtonStyle(display, variant) : undefined}
        class={['embed-frame block min-w-0 cursor-pointer', !display && 'max-h-32']}
      >
        {#if attachment.description}
          <span id={descriptionID(attachment)} class="sr-only">{attachment.description}</span>
        {/if}
        {#if imageAttachmentUrl(attachment)}
          <img
            loading="lazy"
            src={imageAttachmentUrl(attachment)}
            alt={attachment.description || attachment.filename}
            class={[
              display?.fit === 'contain' ? 'object-contain' : 'object-cover',
              display ? 'h-full w-full' : 'max-h-32 w-auto'
            ]}
            onerror={() =>
              refreshAfterAssetError(attachment, attachment.thumbnailUrl ? 'thumbnail' : 'asset')}
          />
        {:else}
          <span class="flex h-16 w-16 items-center justify-center text-muted" aria-hidden="true">
            <span class="iconify icon-[mdi--file-image-outline] text-2xl"></span>
          </span>
        {/if}
      </button>
      {@render attachmentControls(attachment)}
    </div>
  {/snippet}

  {#snippet viewAttachmentButton(attachment: Attachment)}
    <button
      type="button"
      class="btn-secondary attachment-action-button"
      onclick={(event) => {
        // Stop inline playback before the viewer creates another player.
        event.currentTarget
          .closest('[data-attachment-media]')
          ?.querySelectorAll('audio, video')
          .forEach((media) => {
            if (media instanceof HTMLMediaElement) media.pause();
          });
        openAttachmentModal(attachment);
      }}
      aria-label={m('room.attachment.view_label', { filename: attachment.filename })}
      title={m('room.attachment.view_label', { filename: attachment.filename })}
      aria-describedby={attachment.description ? descriptionID(attachment) : undefined}
    >
      <span class="iconify icon-[uil--expand-alt] shrink-0" aria-hidden="true"></span>
    </button>
  {/snippet}

  {#snippet attachmentItem(attachment: Attachment)}
    <div class="flex max-w-full min-w-0 flex-col items-start">
      {#if attachment.videoProcessing && (attachment.contentType === 'image/gif' || attachment.contentType.startsWith('video/'))}
        {@const autoLoop = attachment.contentType === 'image/gif'}
        <div class="group/attachment attachment-video-frame" data-attachment-media>
          {#await loadVideoPlayer(videoPlayerLoadAttempt)}
            <div
              class="embed-frame flex min-h-32 min-w-48 items-center justify-center p-4 text-sm text-muted"
              aria-busy="true"
            >
              {m('common.loading')}
            </div>
          {:then { default: VideoPlayer }}
            <VideoPlayer
              status={attachment.videoProcessing.status}
              variants={attachment.videoProcessing.variants}
              thumbnailUrl={attachment.videoProcessing.thumbnailUrl}
              hlsUrl={attachment.videoProcessing.hlsUrl}
              fallbackUrl={attachment.url}
              fallbackContentType={attachment.contentType}
              width={attachment.videoProcessing.width}
              height={attachment.videoProcessing.height}
              reasonCode={attachment.videoProcessing.reasonCode}
              filename={attachment.filename}
              describedBy={attachment.description ? descriptionID(attachment) : undefined}
              {autoLoop}
              onPosterError={autoLoop
                ? undefined
                : () => refreshAfterAssetError(attachment, 'video')}
              onMediaError={() =>
                refreshAfterAssetError(
                  attachment,
                  !autoLoop && attachment.videoProcessing?.hlsUrl ? 'hls' : 'video'
                )}
            />
          {:catch}
            <div
              class="embed-frame flex min-h-32 min-w-48 flex-col items-center justify-center gap-3 p-4 text-center"
            >
              <p class="text-sm text-muted">{m('common.error.network')}</p>
              <button
                type="button"
                class="btn-secondary"
                onclick={() => (videoPlayerLoadAttempt += 1)}
              >
                {m('common.retry')}
              </button>
            </div>
          {/await}
          {@render attachmentControls(attachment, true)}
        </div>
      {:else if attachment.contentType.startsWith('image/')}
        {@render imageAttachmentButton(attachment, 'single')}
      {:else if attachment.contentType.startsWith('video/') && attachment.url}
        <!--
          A video attachment that hasn't been projected as a processing manifest
          yet — e.g. the message arrived before AssetProcessingStartedEvent did,
          or processing has never been requested for this asset. Render the raw
          original so the user can at least play it.
        -->
        <div class="group/attachment attachment-video-frame embed-frame" data-attachment-media>
          <video
            controls
            preload="metadata"
            src={attachment.url}
            class="max-h-64 max-w-full object-contain"
            onerror={() => refreshAfterAssetError(attachment, 'asset')}
            aria-describedby={attachment.description ? descriptionID(attachment) : undefined}
          >
            <track kind="captions" />
          </video>
          {@render attachmentControls(attachment, true)}
        </div>
      {:else if attachment.contentType.startsWith('audio/') && attachment.url}
        <div
          class="group/attachment embed-frame attachment-card w-[30rem] min-w-0 flex-wrap"
          data-attachment-media
        >
          <audio
            controls
            preload="metadata"
            src={attachment.url}
            class="h-10 max-w-full min-w-[min(12rem,100%)] flex-1 basis-48"
            data-testid="audio-player"
            onerror={() => refreshAfterAssetError(attachment, 'asset')}
            aria-describedby={attachment.description ? descriptionID(attachment) : undefined}
          >
            {attachment.filename}
          </audio>
          {@render attachmentControls(attachment, true, 'row')}
        </div>
      {:else}
        <div
          class="group/attachment embed-frame attachment-card min-w-[min(14rem,100%)]"
        >
          <button
            type="button"
            onclick={() => openAttachmentModal(attachment)}
            aria-label={m('room.attachment.view_label', { filename: attachment.filename })}
            aria-describedby={attachment.description ? descriptionID(attachment) : undefined}
            class="block min-w-0 flex-1 cursor-pointer text-start"
          >
            <div class="flex min-h-10 items-center gap-3">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-6 w-6 shrink-0 text-muted"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
                />
              </svg>
              <span class="min-w-0 text-sm wrap-anywhere"><bdi>{attachment.filename}</bdi></span>
            </div>
          </button>
          {@render attachmentControls(attachment, false, 'row')}
        </div>
      {/if}
      {#if attachment.description && !isGalleryImageAttachment(attachment)}
        <span id={descriptionID(attachment)} class="sr-only">{attachment.description}</span>
      {/if}
    </div>
  {/snippet}

  {#if hasImageGallery}
    <div class="mt-2 flex min-w-0 flex-col gap-2 first:mt-0">
      <div class="relative w-full max-w-full min-w-0">
        <div
          class="w-full overflow-x-auto overscroll-x-contain"
          data-testid="message-image-gallery"
        >
          <div class="flex w-max min-w-full gap-3 p-1" {@attach trackGalleryScrollEdges}>
            {#each imageAttachments as attachment (attachment.id)}
              {@render imageAttachmentButton(attachment, 'gallery')}
            {/each}
          </div>
        </div>
        <div
          aria-hidden="true"
          data-testid="message-image-gallery-left-fade"
          class={[
            'pointer-events-none absolute inset-y-0 left-0 z-10 w-8 bg-gradient-to-r from-background to-transparent transition-opacity',
            !galleryEdges.start && 'opacity-0'
          ]}
        ></div>
        <div
          aria-hidden="true"
          data-testid="message-image-gallery-right-fade"
          class={[
            'pointer-events-none absolute inset-y-0 right-0 z-10 w-8 bg-gradient-to-l from-background to-transparent transition-opacity',
            !galleryEdges.end && 'opacity-0'
          ]}
        ></div>
      </div>

      {#if remainingAttachments.length > 0}
        <div class="flex flex-wrap gap-x-2 gap-y-3">
          {#each remainingAttachments as attachment (attachment.id)}
            {@render attachmentItem(attachment)}
          {/each}
        </div>
      {/if}
    </div>
  {:else}
    <div class="mt-2 flex flex-wrap gap-x-2 gap-y-3 first:mt-0">
      {#each remainingAttachments as attachment (attachment.id)}
        {@render attachmentItem(attachment)}
      {/each}
    </div>
  {/if}
{/if}
