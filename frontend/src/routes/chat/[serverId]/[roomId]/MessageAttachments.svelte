<script lang="ts" module>
  import { graphql } from '$lib/gql';

  export const MessageAttachmentFragment = graphql(`
    fragment MessageAttachmentView on Attachment {
      id
      filename
      contentType
      width
      height
      url
      thumbnailUrl(width: 960, height: 800, fit: CONTAIN)
      videoProcessing {
        status
        durationMs
        width
        height
        thumbnailUrl
        sourceAvailable
        variants {
          url
          quality
          width
          height
          size
        }
        reasonCode
      }
    }
  `);
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- attachment URLs are signed asset URLs */

  import type { FragmentType } from '$lib/gql/fragment-masking';
  import { useFragment } from '$lib/gql/fragment-masking';
  import type { MessageAttachmentViewFragment } from '$lib/gql/graphql';
  import type { ImageItem } from '$lib/ui/ImageModal.svelte';

  type Attachment = MessageAttachmentViewFragment;
  import VideoPlayer from '$lib/components/chat/VideoPlayer.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { pushState } from '$app/navigation';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { toast } from '$lib/ui/toast';
  import { refreshAttachmentUrlsForMessage } from '$lib/attachments/attachmentUrls';

  let {
    attachments: rawAttachments,
    roomId,
    eventId,
    canDeleteAttachment = false
  }: {
    attachments: readonly FragmentType<typeof MessageAttachmentFragment>[];
    roomId: string;
    eventId: string;
    canDeleteAttachment?: boolean;
  } = $props();

  const attachments = $derived(
    rawAttachments.map((a) => useFragment(MessageAttachmentFragment, a))
  );

  const MIN_THUMB_SIZE = 24;

  function thumbSize(w: number, h: number) {
    const isLandscape = w > h;
    const maxW = isLandscape ? 480 : 320;
    const maxH = isLandscape ? 320 : 400;
    const scale = Math.min(maxW / w, maxH / h, 1);
    return {
      width: Math.max(Math.round(w * scale), MIN_THUMB_SIZE),
      height: Math.max(Math.round(h * scale), MIN_THUMB_SIZE)
    };
  }

  const imageAttachments = $derived(attachments.filter((a) => a.contentType.startsWith('image/')));

  const connection = useConnection();

  async function refreshUrlsForMessage(): Promise<Map<string, string>> {
    return refreshAttachmentUrlsForMessage(connection().client, roomId, eventId);
  }

  async function openImageModal(attachment: Attachment) {
    const idx = imageAttachments.indexOf(attachment);
    // Refresh in one round-trip so navigating between images in the
    // lightbox can't hit an expired URL mid-session.
    const freshUrls = await refreshUrlsForMessage();
    const imageItems: ImageItem[] = imageAttachments.map((a) => ({
      id: a.id,
      src: freshUrls.get(a.id) ?? a.url,
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

  async function openDownload(attachment: Attachment, event: MouseEvent) {
    // Intercept the default navigation so we can swap in a fresh URL.
    // The `<a>` keeps its original href as a fallback for middle-click /
    // "Open in new tab", which the browser handles before this runs.
    event.preventDefault();
    const freshUrls = await refreshUrlsForMessage();
    const fresh = freshUrls.get(attachment.id) ?? attachment.url;
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
  <div class="mt-2 flex flex-wrap gap-2 first:mt-0">
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
          />
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="bg-surface-700/80 hover:bg-surface-800 absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full text-white shadow-sm transition-opacity md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
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
          aria-label="View {attachment.filename}"
          class={[
            'group/attachment relative block min-w-0 cursor-pointer embed-frame',
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
          />
          {#if canDeleteAttachment}
            <span
              role="button"
              tabindex="-1"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') openDeleteConfirmation(attachment, e);
              }}
              class="bg-surface-700/80 hover:bg-surface-800 absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full text-white shadow-sm transition-opacity md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
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
          />
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="bg-surface-700/80 hover:bg-surface-800 absolute top-1 right-1 z-10 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full text-white shadow-sm transition-opacity md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
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
        <div class="overflow-hidden rounded-sm">
          <video controls preload="metadata" src={attachment.url} class="max-h-64 max-w-full rounded-sm">
            <track kind="captions" />
          </video>
        </div>
      {:else if attachment.contentType.startsWith('audio/')}
        <div class="group/attachment relative min-w-0">
          <div class="flex items-center gap-3 rounded-lg bg-surface px-3 py-2">
            <audio
              controls
              preload="metadata"
              src={attachment.url}
              class="h-8 max-w-xs"
              data-testid="audio-player"
            >
              <a href={attachment.url}>{attachment.filename}</a>
            </audio>
            <span class="text-sm text-muted">{attachment.filename}</span>
          </div>
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="bg-surface-700/80 hover:bg-surface-800 absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full text-white shadow-sm transition-opacity md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify text-sm uil--times"></span>
            </button>
          {/if}
        </div>
      {:else}
        <a
          href={attachment.url}
          target="_blank"
          rel="noopener noreferrer"
          onclick={(e) => openDownload(attachment, e)}
          aria-label="Download {attachment.filename}"
          class="group/attachment relative block overflow-hidden rounded-lg shadow-md transition-transform"
        >
          <div class="flex h-16 items-center gap-2 rounded-lg bg-surface px-3">
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
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="bg-surface-700/80 hover:bg-surface-800 absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full text-white shadow-sm transition-opacity md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify text-sm uil--times"></span>
            </button>
          {/if}
        </a>
      {/if}
    {/each}
  </div>
{/if}
