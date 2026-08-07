<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import type { ImageViewerModalState } from '$lib/modal';
  import { createAttachmentAPI } from '$lib/api-client/attachments';
  import {
    LIGHTBOX_ATTACHMENT_IMAGE_REFRESH,
    refreshAttachmentUrlsForAssets
  } from '$lib/attachments/attachmentUrls';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import ImageModal from '$lib/ui/ImageModal.svelte';

  let {
    modal,
    onclose
  }: {
    modal: ImageViewerModalState;
    onclose: () => void;
  } = $props();

  const modalIdentity = untrack(() => ({
    serverId: modal.serverId,
    roomId: modal.roomId,
    eventId: modal.eventId
  }));
  let currentIndex = $derived(modal.imageIndex);

  // Preserve roughly an hour of margin ahead of the 23-hour minimum ticket validity.
  const URL_REFRESH_MS = 22 * 60 * 60 * 1000;

  async function refreshUrls() {
    const currentImageItems = modal.imageItems;
    const api = serverConnectionManager
      .getClient(modalIdentity.serverId)
      .getAPI(createAttachmentAPI);
    const freshUrls = await refreshAttachmentUrlsForAssets(
      api,
      modalIdentity.roomId,
      currentImageItems.map((item) => item.id).filter((id): id is string => !!id),
      LIGHTBOX_ATTACHMENT_IMAGE_REFRESH
    );
    if (freshUrls.size === 0) return;

    const latestModal = page.state.modal;
    if (
      latestModal?.type !== 'imageViewer' ||
      latestModal.serverId !== modalIdentity.serverId ||
      latestModal.roomId !== modalIdentity.roomId ||
      latestModal.eventId !== modalIdentity.eventId
    ) {
      return;
    }

    const imageItems = latestModal.imageItems
      .map((item) => {
        const refreshed = item.id ? freshUrls.get(item.id) : undefined;
        return {
          ...item,
          src: refreshed
            ? (assetUrlForServer(modalIdentity.serverId, refreshed.thumbnailAssetUrl?.url) ?? '')
            : item.src,
          originalSrc: refreshed
            ? (assetUrlForServer(modalIdentity.serverId, refreshed.assetUrl?.url) ?? undefined)
            : item.originalSrc
        };
      })
      .filter((item) => item.src !== '');

    if (imageItems.length === 0) {
      onclose();
      return;
    }

    const currentImageId = latestModal.imageItems[currentIndex]?.id;
    const refreshedImageIndex = currentImageId
      ? imageItems.findIndex((item) => item.id === currentImageId)
      : -1;
    replaceState('', {
      ...page.state,
      modal: {
        ...latestModal,
        imageItems,
        imageIndex:
          refreshedImageIndex >= 0
            ? refreshedImageIndex
            : Math.min(latestModal.imageIndex, imageItems.length - 1)
      }
    });
  }

  $effect(() => {
    const interval = window.setInterval(() => {
      refreshUrls().catch((error: unknown) => {
        console.warn('Failed to refresh image viewer URLs', error);
      });
    }, URL_REFRESH_MS);

    return () => window.clearInterval(interval);
  });
</script>

{#if modal.imageItems.length > 0}
  <ImageModal items={modal.imageItems} bind:index={currentIndex} {onclose} />
{/if}
