<!-- @component
Owns one history-backed HTML viewer opening. Consent is component-local and
URL refresh results cannot start a preview or download after this opening ends.
-->
<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { queryClient } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import type { HtmlViewerModalState } from '$lib/modal';
  import { createAttachmentAPI } from '$lib/api-client/attachments';
  import {
    assetUrlNeedsRefresh,
    refreshAttachmentUrlsForAssets
  } from '$lib/attachments/attachmentUrls';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { attachmentDownloadUrl } from '$lib/attachments/attachmentDownloadUrl';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { m } from '$lib/i18n/messages';
  import HtmlAttachmentModal from '$lib/ui/HtmlAttachmentModal.svelte';

  let { modal, onclose }: { modal: HtmlViewerModalState; onclose: () => void } = $props();

  // Only metadata is read on opening. The original document still requires
  // preview consent or an explicit download, and query disposal aborts this read.
  const metadata = createQuery(
    () => {
      const connection = serverConnectionManager.getClient(modal.serverId);
      const { serverId, roomId, attachmentId } = modal;
      return {
        queryKey: [
          ...serverSessionQueryRoot(serverId, connection),
          'attachment-metadata',
          roomId,
          attachmentId
        ],
        queryFn: ({ signal }: { signal: AbortSignal }) =>
          connection.getAPI(createAttachmentAPI).getMetadata(roomId, attachmentId, signal),
        gcTime: 0
      };
    },
    () => queryClient
  );

  let assetUrl = $state.raw(untrack(() => modal.assetUrl));
  let previewUrl = $state<string | null>(null);
  let error = $state<string | null>(null);
  let busy = $state(false);
  let active = true;
  onDestroy(() => {
    active = false;
  });

  const downloadUrl = $derived(
    attachmentDownloadUrl(assetUrlForServer(modal.serverId, assetUrl?.url))
  );

  function isCurrent(target: HtmlViewerModalState): boolean {
    return active && modal === target && page.state.modal === target;
  }

  function close() {
    active = false;
    onclose();
  }

  async function currentUrl(target: HtmlViewerModalState): Promise<string | null> {
    if (!assetUrl?.url || assetUrlNeedsRefresh(assetUrl)) {
      const api = serverConnectionManager.getClient(target.serverId).getAPI(createAttachmentAPI);
      const refreshed = await refreshAttachmentUrlsForAssets(api, target.roomId, [
        target.attachmentId
      ]);
      if (!isCurrent(target)) return null;
      const fresh = refreshed.get(target.attachmentId)?.assetUrl;
      if (!fresh?.url || assetUrlNeedsRefresh(fresh)) throw new Error('Attachment URL unavailable');
      assetUrl = fresh;
    }
    return isCurrent(target) ? assetUrlForServer(target.serverId, assetUrl.url) : null;
  }

  async function showPreview() {
    if (busy) return;
    const target = modal;
    busy = true;
    error = null;
    try {
      const url = await currentUrl(target);
      if (isCurrent(target)) previewUrl = url;
    } catch {
      if (isCurrent(target)) error = m('room.attachment.html_viewer.preview_failed');
    } finally {
      if (isCurrent(target)) busy = false;
    }
  }

  async function download(event: MouseEvent) {
    if (busy || !isCurrent(modal)) {
      event.preventDefault();
      return;
    }
    // Keep valid downloads as native link activations, including modified clicks.
    if (assetUrl?.url && !assetUrlNeedsRefresh(assetUrl)) return;
    event.preventDefault();
    const link = event.currentTarget as HTMLAnchorElement;
    const target = modal;
    busy = true;
    error = null;
    try {
      const url = await currentUrl(target);
      if (!url || !isCurrent(target)) return;
      busy = false;
      await tick();
      if (isCurrent(target) && link.isConnected) link.click();
    } catch {
      if (isCurrent(target)) error = m('room.attachment.download_refresh_failed');
    } finally {
      if (isCurrent(target)) busy = false;
    }
  }
</script>

<HtmlAttachmentModal
  filename={modal.filename}
  contentType={modal.contentType}
  size={metadata.data?.size ?? null}
  sizeLoading={metadata.isPending}
  {downloadUrl}
  {previewUrl}
  {busy}
  {error}
  onpreview={showPreview}
  ondownload={download}
  onclose={close}
/>
