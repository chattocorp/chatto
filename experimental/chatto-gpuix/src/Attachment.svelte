<script lang="ts">
  import PreviewCard from './ui/PreviewCard.svelte';
  import { onDestroy } from 'svelte';
  import type { Server } from '@chatto/client';
  import type { MessageAttachmentView } from '@chatto/client/timeline/messageAttachments';
  import { createAttachmentAPI } from '@chatto/client/api/attachments';
  import {
    assetUrlNeedsRefresh,
    createAssetUrlRetainer,
    DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH
  } from '@chatto/client/attachments/attachmentUrls';
  import { mediaUrl, openMedia } from './media.ts';
  import ActionButton from './ActionButton.svelte';
  import NativeImage from './NativeImage.svelte';

  let {
    attachment,
    server,
    roomId,
    active
  }: {
    attachment: MessageAttachmentView;
    server: Server;
    roomId: string;
    active: boolean;
  } = $props();
  const isImage = $derived(attachment.contentType.startsWith('image/'));
  // SDK refreshes can rotate signatures for unchanged media. Keep the usable
  // URL until expiry or a resource change, as the web attachment renderer does.
  const retainImageUrl = createAssetUrlRetainer();
  const image = $derived(
    retainImageUrl(
      'preview',
      (isImage
        ? (attachment.thumbnailAssetUrl ?? attachment.assetUrl)
        : attachment.videoProcessing?.thumbnailAssetUrl) ?? null
    )
  );
  let error = $state('');
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  /** Use SDK renewal for stale signed URLs instead of reusing an expired thumbnail. */
  async function resolveImage(): Promise<string | null> {
    if (!image || !assetUrlNeedsRefresh(image)) return image?.url ?? null;
    const fresh = await createAttachmentAPI(server.connection.apiConfig).refreshAssetUrls(
      roomId,
      [attachment.id],
      DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH
    );
    const urls = fresh.get(attachment.id);
    return (
      (isImage ? (urls?.thumbnailAssetUrl ?? urls?.assetUrl) : urls?.videoThumbnailAssetUrl)?.url ??
      null
    );
  }

  async function open() {
    error = '';
    try {
      let asset = attachment.assetUrl;
      if (!asset || assetUrlNeedsRefresh(asset)) {
        const fresh = await createAttachmentAPI(server.connection.apiConfig).refreshAssetUrls(
          roomId,
          [attachment.id],
          DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH
        );
        asset = fresh.get(attachment.id)?.assetUrl;
      }
      if (!alive) return;
      const url = mediaUrl(asset?.url, server.serverUrl);
      if (!url) throw new Error('Unavailable');
      await openMedia(url);
    } catch {
      if (alive) error = 'Could not open attachment. Try again.';
    }
  }
</script>

<PreviewCard>
  {#if image}
    <NativeImage
      src={image.url}
      serverUrl={server.serverUrl}
      {active}
      alt={attachment.description || attachment.filename}
      resolveSource={resolveImage}
    />
  {/if}
  <text class="filename">{attachment.filename}</text>
  {#if attachment.description}<text class="description">{attachment.description}</text>{/if}
  <text class="kind"
    >{attachment.contentType}{attachment.videoProcessing
      ? ` · ${attachment.videoProcessing.status.toLowerCase()}`
      : ''}</text
  >
  <ActionButton
    quiet
    label={isImage ? 'Open full image' : 'Open attachment'}
    onclick={() => void open()}
  />
  {#if error}<text class="error">{error}</text>{/if}
</PreviewCard>

<style>
  .filename {
    font-size: 13px;
    font-weight: bold;
  }
  .description {
    font-size: 13px;
    white-space: pre-wrap;
    user-select: text;
  }
  .kind {
    font-size: 11px;
    color: var(--color-muted);
  }
  .error {
    font-size: 12px;
    color: var(--color-error);
  }
</style>
