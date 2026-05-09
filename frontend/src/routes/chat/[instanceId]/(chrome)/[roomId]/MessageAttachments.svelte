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
        variants {
          url
          quality
          width
          height
          size
        }
        errorMessage
      }
    }
  `);
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- external attachment URLs */
  import type { FragmentType } from '$lib/gql/fragment-masking';
  import { useFragment } from '$lib/gql/fragment-masking';
  import type { MessageAttachmentViewFragment } from '$lib/gql/graphql';
  import type { ImageItem } from '$lib/ui/ImageModal.svelte';

  type Attachment = MessageAttachmentViewFragment;
  import VideoPlayer from '$lib/components/chat/VideoPlayer.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { pushState } from '$app/navigation';

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
  const imageItems: ImageItem[] = $derived(
    imageAttachments.map((a) => ({ src: a.url, alt: a.filename, filename: a.filename }))
  );

  function openImageModal(attachment: Attachment) {
    const idx = imageAttachments.indexOf(attachment);
    pushState('', {
      modal: {
        type: 'imageViewer',
        imageItems: imageItems,
        imageIndex: idx >= 0 ? idx : 0
      }
    });
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
        <div class="group/attachment relative">
          <VideoPlayer
            status={attachment.videoProcessing.status}
            variants={attachment.videoProcessing.variants}
            thumbnailUrl={attachment.videoProcessing.thumbnailUrl}
            width={attachment.videoProcessing.width}
            height={attachment.videoProcessing.height}
            errorMessage={attachment.videoProcessing.errorMessage}
            filename={attachment.filename}
            autoLoop
          />
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full bg-surface-700/80 text-white shadow-sm transition-opacity hover:bg-surface-800 md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify uil--times text-sm"></span>
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
            'group/attachment embed-frame relative block cursor-pointer',
            !size && 'max-h-64'
          ]}
          style={size ? `width: ${size.width}px; height: ${size.height}px` : undefined}
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
              class="absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full bg-surface-700/80 text-white shadow-sm transition-opacity hover:bg-surface-800 md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify uil--times text-sm"></span>
            </span>
          {/if}
        </button>
      {:else if attachment.contentType.startsWith('video/') && attachment.videoProcessing}
        <div class="group/attachment relative">
          <VideoPlayer
            status={attachment.videoProcessing.status}
            variants={attachment.videoProcessing.variants}
            thumbnailUrl={attachment.videoProcessing.thumbnailUrl}
            width={attachment.videoProcessing.width}
            height={attachment.videoProcessing.height}
            errorMessage={attachment.videoProcessing.errorMessage}
            filename={attachment.filename}
          />
          {#if canDeleteAttachment}
            <button
              type="button"
              onclick={(e) => openDeleteConfirmation(attachment, e)}
              class="absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full bg-surface-700/80 text-white shadow-sm transition-opacity hover:bg-surface-800 md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify uil--times text-sm"></span>
            </button>
          {/if}
        </div>
      {:else if attachment.contentType.startsWith('video/')}
        <!-- Video without processing data — original may have been deleted after transcoding -->
        <div class="flex h-16 items-center gap-2 rounded-lg bg-surface px-3">
          <span class="iconify uil--video text-lg text-muted"></span>
          <span class="text-sm text-muted">Video unavailable</span>
        </div>
      {:else if attachment.contentType.startsWith('audio/')}
        <div class="group/attachment relative">
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
              class="absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full bg-surface-700/80 text-white shadow-sm transition-opacity hover:bg-surface-800 md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify uil--times text-sm"></span>
            </button>
          {/if}
        </div>
      {:else}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external asset URL -->
        <a
          href={attachment.url}
          target="_blank"
          rel="noopener noreferrer"
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
              class="absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full bg-surface-700/80 text-white shadow-sm transition-opacity hover:bg-surface-800 md:opacity-0 md:group-hover/attachment:opacity-100"
              aria-label="Delete attachment"
              title="Delete attachment"
            >
              <span class="iconify uil--times text-sm"></span>
            </button>
          {/if}
        </a>
      {/if}
    {/each}
  </div>
{/if}
