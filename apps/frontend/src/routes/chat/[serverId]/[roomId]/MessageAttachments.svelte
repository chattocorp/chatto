<script lang="ts" module>
  import { MessageAttachmentViewDocument } from '$lib/render/types';

  export const MessageAttachmentViewData = MessageAttachmentViewDocument;
</script>

<script lang="ts">
  import type { RenderType } from '$lib/render/data';
  import { useRenderData } from '$lib/render/data';
  import type { MessageAttachmentView } from '$lib/render/types';
  import type { ImageItem } from '$lib/ui/ImageModal.svelte';

  type RawAttachment = MessageAttachmentView;
  import VideoPlayer from '$lib/components/chat/VideoPlayer.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { pushState } from '$app/navigation';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import * as m from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import {
    assetUrlNeedsRefresh,
    earliestAssetUrlRefreshAt,
    mergeRefreshedAttachmentUrls,
    refreshAttachmentUrlsForMessage,
    withAssetUrlRetryParam,
    type ExpiringAssetUrl,
    type RefreshedAttachmentUrls
  } from '$lib/attachments/attachmentUrls';
  import { createAttachmentAPI } from '$lib/api/attachments';
  import { assetUrlForServer } from '$lib/assets/assetUrls';

  let {
    attachments: rawAttachments,
    serverId,
    roomId,
    eventId,
    canDeleteAttachment = false
  }: {
    attachments: readonly RenderType<typeof MessageAttachmentViewData>[];
    serverId: string;
    roomId: string;
    eventId: string;
    canDeleteAttachment?: boolean;
  } = $props();

  let refreshedAttachmentUrls = $state.raw(new Map<string, RefreshedAttachmentUrls>());
  const assetRetrySalts = new SvelteMap<string, number>();
  let refreshPromise: Promise<Map<string, RefreshedAttachmentUrls>> | null = null;
  const failedAssetRefreshKeys = new SvelteSet<string>();

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

  function normalizeAttachment(attachment: RawAttachment) {
    const refreshed = refreshedAttachmentUrls.get(attachment.id);
    const assetUrl = withRetrySalt(
      normalizeAssetUrl(refreshed?.assetUrl ?? attachment.assetUrl),
      attachment.id,
      'asset'
    );
    const thumbnailAssetUrl = withRetrySalt(
      normalizeAssetUrl(refreshed?.thumbnailAssetUrl ?? attachment.thumbnailAssetUrl),
      attachment.id,
      'thumbnail'
    );
    const videoThumbnailAssetUrl = withRetrySalt(
      normalizeAssetUrl(
        refreshed?.videoThumbnailAssetUrl ?? attachment.videoProcessing?.thumbnailAssetUrl
      ),
      attachment.id,
      'video'
    );

    return {
      ...attachment,
      assetUrl,
      url: assetUrl?.url ?? '',
      thumbnailAssetUrl,
      thumbnailUrl: thumbnailAssetUrl?.url ?? null,
      videoProcessing: attachment.videoProcessing
        ? {
            ...attachment.videoProcessing,
            thumbnailAssetUrl: videoThumbnailAssetUrl,
            thumbnailUrl: videoThumbnailAssetUrl?.url ?? null,
            variants: attachment.videoProcessing.variants.map((variant) => {
              const variantAssetUrl = withRetrySalt(
                normalizeAssetUrl(
                  refreshed?.variantAssetUrls.get(variant.quality) ?? variant.assetUrl
                ),
                attachment.id,
                'video'
              );
              return {
                ...variant,
                assetUrl: variantAssetUrl,
                url: variantAssetUrl?.url ?? ''
              };
            })
          }
        : null
    };
  }

  type Attachment = ReturnType<typeof normalizeAttachment>;

  const attachments = $derived.by(() =>
    rawAttachments.map((a) => normalizeAttachment(useRenderData(MessageAttachmentViewData, a)))
  );

  const MIN_THUMB_SIZE = 24;
  const EXTREME_ASPECT_RATIO = 3;
  const LANDSCAPE_THUMB_MAX_WIDTH = 480;
  const LANDSCAPE_THUMB_MAX_HEIGHT = 320;
  const PORTRAIT_THUMB_MAX_WIDTH = 320;
  const PORTRAIT_THUMB_MAX_HEIGHT = 400;

  function thumbSize(w: number, h: number) {
    const isLandscape = w > h;
    const aspectRatio = w / h;
    const maxW = isLandscape ? LANDSCAPE_THUMB_MAX_WIDTH : PORTRAIT_THUMB_MAX_WIDTH;
    const maxH = isLandscape ? LANDSCAPE_THUMB_MAX_HEIGHT : PORTRAIT_THUMB_MAX_HEIGHT;

    if (aspectRatio >= EXTREME_ASPECT_RATIO || aspectRatio <= 1 / EXTREME_ASPECT_RATIO) {
      return {
        width: maxW,
        height: maxH
      };
    }

    const scale = Math.min(maxW / w, maxH / h, 1);
    return {
      width: Math.max(Math.round(w * scale), MIN_THUMB_SIZE),
      height: Math.max(Math.round(h * scale), MIN_THUMB_SIZE)
    };
  }

  const imageAttachments = $derived(attachments.filter((a) => a.contentType.startsWith('image/')));

  const connection = useConnection();

  function attachmentAssetUrls(attachment: Attachment) {
    return [
      attachment.assetUrl,
      attachment.thumbnailAssetUrl,
      attachment.videoProcessing?.thumbnailAssetUrl,
      ...(attachment.videoProcessing?.variants.map((variant) => variant.assetUrl) ?? [])
    ];
  }

  const nextAssetUrlRefreshAt = $derived.by(() => {
    return earliestAssetUrlRefreshAt(
      attachments.flatMap((attachment) => attachmentAssetUrls(attachment))
    );
  });

  $effect(() => {
    if (nextAssetUrlRefreshAt === null) return;

    const timeout = window.setTimeout(
      () => {
        refreshAndApplyUrls().catch((error: unknown) => {
          console.warn('Failed to refresh attachment URLs before expiry', error);
        });
      },
      Math.max(0, nextAssetUrlRefreshAt - Date.now())
    );

    return () => window.clearTimeout(timeout);
  });

  function hasRefreshableStaleUrl() {
    return attachments.some((attachment) =>
      attachmentAssetUrls(attachment).some((assetUrl) => assetUrlNeedsRefresh(assetUrl))
    );
  }

  function refreshStaleUrls() {
    if (!hasRefreshableStaleUrl()) return;
    refreshAndApplyUrls().catch((error: unknown) => {
      console.warn('Failed to refresh stale attachment URLs', error);
    });
  }

  function handleVisibilityChange() {
    if (document.visibilityState === 'visible') {
      refreshStaleUrls();
    }
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

  function refreshAfterAssetError(attachment: Attachment, role: string) {
    const key = `${attachment.id}:${role}`;
    if (failedAssetRefreshKeys.has(key)) return;
    failedAssetRefreshKeys.add(key);
    refreshAndApplyUrls()
      .then(() => {
        assetRetrySalts.set(key, Date.now());
      })
      .catch((error: unknown) => {
        console.warn('Failed to refresh attachment URL after load error', error);
      });
  }

  $effect(() => {
    if (hasRefreshableStaleUrl()) {
      refreshAndApplyUrls().catch((error: unknown) => {
        console.warn('Failed to refresh stale attachment URLs', error);
      });
    }
  });

  $effect(() => {
    window.addEventListener('focus', refreshStaleUrls);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      window.removeEventListener('focus', refreshStaleUrls);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  });

  async function refreshUrlsForMessage(): Promise<Map<string, RefreshedAttachmentUrls>> {
    const conn = connection();
    return refreshAttachmentUrlsForMessage(
      createAttachmentAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }),
      roomId,
      eventId
    );
  }

  async function openImageModal(attachment: Attachment) {
    const idx = imageAttachments.indexOf(attachment);
    // Refresh in one round-trip so navigating between images in the
    // lightbox can't hit an expired URL mid-session.
    const freshUrls = await refreshAndApplyUrls();
    const imageItems: ImageItem[] = imageAttachments.map((a) => ({
      id: a.id,
      src: normalizeAssetUrl(freshUrls.get(a.id)?.assetUrl)?.url ?? a.url,
      alt: a.filename,
      filename: a.filename
    }));
    pushState('', {
      modal: {
        type: 'imageViewer',
        roomId,
        eventId,
        imageItems,
        imageIndex: idx >= 0 ? idx : 0
      }
    });
  }

  async function openDownload(attachment: Attachment) {
    const freshUrls = await refreshAndApplyUrls();
    const fresh = normalizeAssetUrl(freshUrls.get(attachment.id)?.assetUrl)?.url ?? attachment.url;
    if (!fresh) {
      toast.error('Could not refresh download link');
      return;
    }
    window.open(fresh, '_blank', 'noopener,noreferrer');
  }

  function openDeleteConfirmation(attachment: Attachment, event: Event) {
    // Prevent opening the image modal
    event.stopPropagation();

    pushState('', {
      modal: {
        type: 'deleteAttachment',
        roomId,
        eventId,
        attachmentId: attachment.id,
        attachmentFilename: attachment.filename
      }
    });
  }
</script>

{#if attachments.length > 0}
  <div class="mt-2 flex flex-wrap gap-x-2 gap-y-3 first:mt-0">
    {#each attachments as attachment (attachment.id)}
      {#if attachment.contentType === 'image/gif' && attachment.videoProcessing}
        <div class="group/attachment relative min-w-0">
          <VideoPlayer
            status={attachment.videoProcessing.status}
            variants={attachment.videoProcessing.variants}
            thumbnailUrl={attachment.videoProcessing.thumbnailUrl}
            width={attachment.videoProcessing.width}
            height={attachment.videoProcessing.height}
            reasonCode={attachment.videoProcessing.reasonCode}
            filename={attachment.filename}
            autoLoop
            onMediaError={() => refreshAfterAssetError(attachment, 'video')}
          />
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="embed-control-button md:group-hover/attachment:opacity-100"
              aria-label={m['room.attachment.delete_label']()}
              title={m['room.attachment.delete_label']()}
            >
              <span class="iconify text-sm uil--times"></span>
            </button>
          {/if}
        </div>
      {:else if attachment.contentType.startsWith('image/')}
        {@const size =
          attachment.width && attachment.height
            ? thumbSize(attachment.width, attachment.height)
            : null}
        <button
          type="button"
          onclick={() => openImageModal(attachment)}
          aria-label={m['room.attachment.view_label']({ filename: attachment.filename })}
          class={[
            'group/attachment relative embed-frame block min-w-0 cursor-pointer',
            !size && 'max-h-64'
          ]}
          style={size
            ? `width: ${size.width}px; max-width: 100%; aspect-ratio: ${size.width} / ${size.height}`
            : undefined}
        >
          <SkeletonImg
            loading="lazy"
            src={attachment.thumbnailUrl ?? attachment.url}
            alt={attachment.filename}
            class={['object-cover', size ? 'h-full w-full' : 'max-h-64 w-auto']}
            onerror={() =>
              refreshAfterAssetError(attachment, attachment.thumbnailUrl ? 'thumbnail' : 'asset')}
          />
          {#if canDeleteAttachment}
            <span
              role="button"
              tabindex="-1"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') openDeleteConfirmation(attachment, e);
              }}
              class="embed-control-button md:group-hover/attachment:opacity-100"
              aria-label={m['room.attachment.delete_label']()}
              title={m['room.attachment.delete_label']()}
            >
              <span class="iconify text-sm uil--times"></span>
            </span>
          {/if}
        </button>
      {:else if attachment.contentType.startsWith('video/') && attachment.videoProcessing}
        <div class="group/attachment relative min-w-0">
          <VideoPlayer
            status={attachment.videoProcessing.status}
            variants={attachment.videoProcessing.variants}
            thumbnailUrl={attachment.videoProcessing.thumbnailUrl}
            width={attachment.videoProcessing.width}
            height={attachment.videoProcessing.height}
            reasonCode={attachment.videoProcessing.reasonCode}
            filename={attachment.filename}
            onMediaError={() => refreshAfterAssetError(attachment, 'video')}
          />
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="embed-control-button z-10 md:group-hover/attachment:opacity-100"
              aria-label={m['room.attachment.delete_label']()}
              title={m['room.attachment.delete_label']()}
            >
              <span class="iconify text-sm uil--times"></span>
            </button>
          {/if}
        </div>
      {:else if attachment.contentType.startsWith('video/')}
        <!--
          A video attachment that hasn't been projected as a processing manifest
          yet — e.g. the message arrived before AssetProcessingStartedEvent did,
          or processing has never been requested for this asset. Render the raw
          original so the user can at least play it.
        -->
        <div class="embed-frame">
          <video
            controls
            preload="metadata"
            src={attachment.url}
            class="max-h-64 max-w-full"
            onerror={() => refreshAfterAssetError(attachment, 'asset')}
          >
            <track kind="captions" />
          </video>
        </div>
      {:else if attachment.contentType.startsWith('audio/')}
        <div class="group/attachment relative min-w-0">
          <div class="embed-frame flex items-center gap-3 px-3 py-2">
            <audio
              controls
              preload="metadata"
              src={attachment.url}
              class="h-8 max-w-xs"
              data-testid="audio-player"
              onerror={() => refreshAfterAssetError(attachment, 'asset')}
            >
              {attachment.filename}
            </audio>
            <span class="text-sm text-muted">{attachment.filename}</span>
          </div>
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="embed-control-button md:group-hover/attachment:opacity-100"
              aria-label={m['room.attachment.delete_label']()}
              title={m['room.attachment.delete_label']()}
            >
              <span class="iconify text-sm uil--times"></span>
            </button>
          {/if}
        </div>
      {:else}
        <div class="group/attachment relative embed-frame block">
          <button
            type="button"
            onclick={() => openDownload(attachment)}
            aria-label={m['room.attachment.download_label']({ filename: attachment.filename })}
            class="block w-full cursor-pointer text-left"
          >
            <div class="flex h-16 items-center gap-2 px-3">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-6 w-6 text-muted"
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
              <span class="text-sm">{attachment.filename}</span>
            </div>
          </button>
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="embed-control-button md:group-hover/attachment:opacity-100"
              aria-label={m['room.attachment.delete_label']()}
              title={m['room.attachment.delete_label']()}
            >
              <span class="iconify text-sm uil--times"></span>
            </button>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
{/if}
