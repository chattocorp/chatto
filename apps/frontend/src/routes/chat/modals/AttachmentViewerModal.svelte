<!-- @component
Owns one viewer opening. Selection changes fence all pending work and unmount
media. HTML consent is never stored in history or carried to another selection.
-->
<script lang="ts">
  import { onMount, onDestroy, tick, untrack } from 'svelte';
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { queryClient } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import type { AttachmentViewerModalState } from '$lib/modal';
  import type { MessageAttachmentView } from '$lib/render/messageAttachments';
  import { isHtmlAttachment } from '$lib/render/messageAttachments';
  import { createAttachmentAPI } from '$lib/api-client/attachments';
  import {
    assetUrlNeedsRefresh,
    LIGHTBOX_ATTACHMENT_IMAGE_REFRESH,
    withAssetUrlRetryParam,
    refreshAttachmentUrlsForAssets
  } from '$lib/attachments/attachmentUrls';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { attachmentDownloadUrl } from '$lib/attachments/attachmentDownloadUrl';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { m } from '$lib/i18n/messages';
  import AttachmentModal from '$lib/ui/AttachmentModal.svelte';
  import AttachmentPreview from '$lib/ui/AttachmentPreview.svelte';

  let { modal, onclose }: { modal: AttachmentViewerModalState; onclose: () => void } = $props();
  let index = $state(untrack(() => Math.max(0, Math.min(modal.index, modal.items.length - 1))));
  let item = $state.raw<MessageAttachmentView>(untrack(() => modal.items[index]));
  let previewUrl = $state<string | null>(null);
  let error = $state<string | null>(null);
  let busy = $state(false);
  let generation = $state(0);
  let active = true;
  let recoveryUsed = false;
  let retryAttempt = 0;
  const html = $derived(isHtmlAttachment(item.contentType));
  const type = $derived(item.contentType.split(';', 1)[0].trim().toLowerCase());
  const previewable = $derived(html || /^(image|audio|video)\//.test(type));
  const zoomable = $derived(type.startsWith('image/') && !item.videoProcessing);
  const downloadUrl = $derived(
    attachmentDownloadUrl(assetUrlForServer(modal.serverId, item.assetUrl?.url))
  );
  const metadata = createQuery(
    () => {
      const connection = serverConnectionManager.getClient(modal.serverId);
      const { serverId, roomId } = modal;
      const assetId = item.id;
      return {
        queryKey: [
          ...serverSessionQueryRoot(serverId, connection),
          'attachment-metadata',
          roomId,
          assetId
        ],
        queryFn: ({ signal }: { signal: AbortSignal }) =>
          connection.getAPI(createAttachmentAPI).getMetadata(roomId, assetId, signal),
        gcTime: 0
      };
    },
    () => queryClient
  );

  function current(target: AttachmentViewerModalState, selected: number) {
    return active && modal === target && page.state.modal === target && generation === selected;
  }
  onDestroy(() => {
    active = false;
  });
  onMount(() => {
    if (!html && previewable) void showPreview();
  });

  function close() {
    active = false;
    onclose();
  }
  function navigate(direction: -1 | 1) {
    generation += 1;
    index = (index + direction + modal.items.length) % modal.items.length;
    item = modal.items[index];
    previewUrl = null;
    error = null;
    busy = false;
    recoveryUsed = false;
    if (
      !isHtmlAttachment(item.contentType) &&
      /^(image|audio|video)\//i.test(item.contentType.trim())
    )
      void showPreview();
  }

  async function refresh(target: AttachmentViewerModalState, selected: number) {
    const original = item;
    const api = serverConnectionManager.getClient(target.serverId).getAPI(createAttachmentAPI);
    const result = (
      await refreshAttachmentUrlsForAssets(
        api,
        target.roomId,
        [original.id],
        LIGHTBOX_ATTACHMENT_IMAGE_REFRESH
      )
    ).get(original.id);
    if (!current(target, selected)) return false;
    if (!result?.assetUrl?.url || assetUrlNeedsRefresh(result.assetUrl))
      throw new Error('Attachment URL unavailable');
    item = {
      ...original,
      assetUrl: result.assetUrl,
      thumbnailAssetUrl: result.thumbnailAssetUrl,
      videoProcessing: original.videoProcessing
        ? {
            ...original.videoProcessing,
            thumbnailAssetUrl: result.videoThumbnailAssetUrl,
            hlsMasterPlaylistUrl: result.hlsMasterPlaylistUrl,
            variants: original.videoProcessing.variants.map((v) => ({
              ...v,
              assetUrl: result.variantAssetUrls.get(v.quality) ?? null
            }))
          }
        : null
    };
    return true;
  }

  async function showPreview(force = false) {
    if (busy) return;
    const target = modal,
      selected = generation;
    busy = true;
    error = null;
    try {
      // Refresh transformed image URLs at viewer resolution. Original HTML
      // bytes remain untouched until the consent button calls this method.
      if (
        force ||
        !item.assetUrl?.url ||
        assetUrlNeedsRefresh(item.assetUrl) ||
        type.startsWith('image/') ||
        [
          item.videoProcessing?.hlsMasterPlaylistUrl,
          item.videoProcessing?.thumbnailAssetUrl,
          ...(item.videoProcessing?.variants.map((v) => v.assetUrl) ?? [])
        ].some((value) => assetUrlNeedsRefresh(value))
      ) {
        if (!(await refresh(target, selected))) return;
      }
      if (current(target, selected)) {
        const source =
          type.startsWith('image/') && !item.videoProcessing
            ? (item.thumbnailAssetUrl?.url ?? item.assetUrl?.url)
            : item.assetUrl?.url;
        const resolved = assetUrlForServer(target.serverId, source);
        previewUrl =
          force && resolved ? withAssetUrlRetryParam(resolved, ++retryAttempt) : resolved;
      }
    } catch {
      if (current(target, selected)) {
        previewUrl = null;
        error = m('room.attachment.html_viewer.preview_failed');
      }
    } finally {
      if (current(target, selected)) busy = false;
    }
  }

  async function recoverMedia(): Promise<string | null> {
    if (busy) return null;
    if (recoveryUsed) {
      error = m('room.attachment.html_viewer.preview_failed');
      return null;
    }
    recoveryUsed = true;
    const target = modal,
      selected = generation;
    await showPreview(true);
    return current(target, selected)
      ? (assetUrlForServer(target.serverId, item.videoProcessing?.hlsMasterPlaylistUrl?.url) ??
          previewUrl)
      : null;
  }

  async function download(event: MouseEvent) {
    const target = modal,
      selected = generation;
    if (busy || !current(target, selected)) {
      event.preventDefault();
      return;
    }
    if (item.assetUrl?.url && !assetUrlNeedsRefresh(item.assetUrl)) return;
    event.preventDefault();
    const link = event.currentTarget as HTMLAnchorElement;
    busy = true;
    error = null;
    try {
      if (!(await refresh(target, selected))) return;
      busy = false;
      await tick();
      if (current(target, selected) && link.isConnected) link.click();
    } catch {
      if (current(target, selected)) error = m('room.attachment.download_refresh_failed');
    } finally {
      if (current(target, selected)) busy = false;
    }
  }
</script>

<AttachmentModal
  filename={item.filename}
  contentType={item.contentType}
  description={item.description ?? undefined}
  size={metadata.data?.size ?? null}
  sizeLoading={metadata.isPending}
  {index}
  count={modal.items.length}
  onnavigate={navigate}
  {downloadUrl}
  {busy}
  imageViewer={zoomable}
  {error}
  ondownload={download}
  onretry={!html && previewable
    ? () => {
        recoveryUsed = false;
        void showPreview(true);
      }
    : undefined}
  onclose={close}
>
  {#key generation}
    <AttachmentPreview
      {item}
      serverId={modal.serverId}
      url={previewUrl}
      {busy}
      {zoomable}
      onpreview={() => {
        recoveryUsed = false;
        void showPreview();
      }}
      onerror={recoverMedia}
    />
  {/key}
</AttachmentModal>
