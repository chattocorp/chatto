<!-- @component
Owns one viewer opening. Selection changes fence all pending work and unmount
media. HTML consent is never stored in history or carried to another selection.
-->
<script lang="ts">
  import { onMount, onDestroy, tick, untrack } from 'svelte';
  import { page } from '$app/state';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import type { AttachmentViewerModalState } from '$lib/modal';
  import type { MessageAttachmentView } from '@chatto/client/timeline/messageAttachments';
  import { isHtmlAttachment } from '@chatto/client/timeline/messageAttachments';
  import { createAttachmentAPI } from '@chatto/client/api/attachments';
  import {
    assetUrlNeedsRefresh,
    LIGHTBOX_ATTACHMENT_IMAGE_REFRESH,
    withAssetUrlRetryParam,
    refreshAttachmentUrlsForAssets
  } from '@chatto/client/attachments/attachmentUrls';
  import { assetUrlForServer } from '@chatto/client/util/assetUrls';
  import { attachmentDownloadUrl } from '$lib/attachments/attachmentDownloadUrl';
  import { isMarkdownAttachment } from '$lib/attachments/isMarkdownAttachment';
  import { serverConnectionManager } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import { AttachmentModal, AttachmentPreview } from '$lib/ui/attachments';

  let { modal, onclose }: { modal: AttachmentViewerModalState; onclose: () => void } = $props();
  let index = $state(untrack(() => Math.max(0, Math.min(modal.index, modal.items.length - 1))));
  let item = $state.raw<MessageAttachmentView>(untrack(() => modal.items[index]));
  let previewUrl = $state<string | null>(null);
  let markdownHtml = $state<string | null>(null);
  let error = $state<string | null>(null);
  let busy = $state(false);
  let generation = $state(0);
  let active = true;
  let recoveryUsed = false;
  let retryAttempt = 0;
  let previewRequest: AbortController | null = null;
  const html = $derived(isHtmlAttachment(item.contentType));
  const markdown = $derived(isMarkdownAttachment(item.contentType, item.filename));
  const type = $derived(item.contentType.split(';', 1)[0].trim().toLowerCase());
  const previewable = $derived(html || markdown || /^(image|audio|video)\//.test(type));
  const zoomable = $derived(type.startsWith('image/') && !item.videoProcessing);
  const downloadUrl = $derived(
    attachmentDownloadUrl(assetUrlForServer(modal.serverId, item.assetUrl?.url))
  );
  const metadata = createQuery(() => {
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
  });

  function current(target: AttachmentViewerModalState, selected: number) {
    return active && modal === target && page.state.modal === target && generation === selected;
  }
  onDestroy(() => {
    active = false;
    clearDocument();
  });
  onMount(() => {
    if (!html && previewable) void showPreview();
  });

  function close() {
    active = false;
    clearDocument();
    onclose();
  }
  /** Cancel document reads and remove rendered content at the viewer boundary. */
  function clearDocument() {
    previewRequest?.abort();
    previewRequest = null;
    markdownHtml = null;
  }
  function navigate(direction: -1 | 1) {
    generation += 1;
    clearDocument();
    index = (index + direction + modal.items.length) % modal.items.length;
    item = modal.items[index];
    previewUrl = null;
    error = null;
    busy = false;
    recoveryUsed = false;
    if (!html && previewable) void showPreview();
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
    markdownHtml = null;
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
      if (!current(target, selected)) return;
      if (markdown) {
        const controller = new AbortController();
        previewRequest = controller;
        let body: string;
        try {
          body = await readMarkdown(controller.signal, force);
        } catch {
          // A signed URL can expire between selection and the byte request.
          // Recover once, then leave further retries to the viewer control.
          if (!current(target, selected) || controller.signal.aborted) return;
          if (!(await refresh(target, selected))) return;
          body = await readMarkdown(controller.signal, true);
        }
        if (!current(target, selected)) return;
        const { renderMarkdown } = await import('$lib/markdown');
        const rendered = await renderMarkdown(body, { mentions: false });
        if (current(target, selected)) markdownHtml = rendered;
      } else {
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
      if (current(target, selected)) {
        busy = false;
        previewRequest = null;
      }
    }
  }

  /** Fetch only the original asset; document links do not load during rendering. */
  async function readMarkdown(signal: AbortSignal, retry = false): Promise<string> {
    const source = assetUrlForServer(modal.serverId, item.assetUrl?.url);
    if (!source) throw new Error('Attachment URL unavailable');
    const response = await fetch(retry ? withAssetUrlRetryParam(source, ++retryAttempt) : source, {
      signal,
      credentials: 'omit',
      referrerPolicy: 'no-referrer'
    });
    if (!response.ok) throw new Error('Attachment request failed');
    return response.text();
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
      {markdownHtml}
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
