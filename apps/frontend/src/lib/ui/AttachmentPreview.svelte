<!-- @component
Preview-only content. The owner decides when URLs can be used and resets this
component on selection changes so playback and consent cannot cross items.
-->
<script lang="ts">
  import { scale } from 'svelte/transition';
  import { expoOutTransition } from './motion';
  import { m } from '$lib/i18n/messages';
  import ZoomableImage from './ZoomableImage.svelte';
  import LoadingFog from './LoadingFog.svelte';
  import type { MessageAttachmentView } from '$lib/render/messageAttachments';
  import { isHtmlAttachment } from '$lib/render/messageAttachments';
  import { assetUrlForServer } from '$lib/assets/assetUrls';

  let {
    item,
    serverId,
    url,
    busy,
    zoomable = false,
    onpreview,
    onerror
  }: {
    item: MessageAttachmentView;
    serverId: string;
    url: string | null;
    busy: boolean;
    /** Static image previews can use the viewer's zoom surface. */
    zoomable?: boolean;
    onpreview: () => void;
    onerror: () => Promise<string | null>;
  } = $props();
  const type = $derived(item.contentType.split(';', 1)[0].trim().toLowerCase());
  const html = $derived(isHtmlAttachment(type));
  const id = $props.id();
</script>

{#if html && !url}
  <div class="flex max-h-full flex-col items-center gap-3 overflow-y-auto p-4 text-center">
    <button
      type="button"
      class="btn-action"
      disabled={busy}
      aria-describedby={`${id}-warning`}
      onclick={onpreview}
    >
      {busy ? m('common.loading') : m('room.attachment.html_viewer.show_preview')}
    </button>
    <p id={`${id}-warning`} class="max-w-md text-sm text-muted">
      {m('room.attachment.html_viewer.warning')}
    </p>
  </div>
{:else if url && html}
  <iframe
    src={url}
    title={m('room.attachment.html_viewer.preview_title', { filename: item.filename })}
    sandbox=""
    referrerpolicy="no-referrer"
    class="h-full w-full rounded-md border-0 bg-white"
    in:scale={{ start: 0.98, ...expoOutTransition() }}
  ></iframe>
{:else if url && item.videoProcessing?.status === 'COMPLETED' && (type.startsWith('video/') || type === 'image/gif')}
  {#await import('$lib/components/chat/VideoPlayer.svelte')}
    <LoadingFog class="h-full min-h-32 w-full" />
  {:then { default: VideoPlayer }}
    <VideoPlayer
      viewer
      status={item.videoProcessing.status}
      variants={item.videoProcessing.variants
        .map((v) => ({ ...v, url: assetUrlForServer(serverId, v.assetUrl?.url) ?? '' }))
        .filter((v) => v.url)}
      thumbnailUrl={assetUrlForServer(serverId, item.videoProcessing.thumbnailAssetUrl?.url)}
      hlsUrl={assetUrlForServer(serverId, item.videoProcessing.hlsMasterPlaylistUrl?.url)}
      fallbackUrl={url}
      fallbackContentType={type}
      width={item.videoProcessing.width}
      height={item.videoProcessing.height}
      reasonCode={item.videoProcessing.reasonCode}
      filename={item.filename}
      autoLoop={type === 'image/gif'}
      onMediaError={onerror}
      onPosterError={() => {
        void onerror();
      }}
    />
  {:catch}
    <p role="alert">{m('room.attachment.html_viewer.preview_failed')}</p>
  {/await}
{:else if url && type.startsWith('image/')}
  {#if zoomable}
    <ZoomableImage
      src={url}
      alt={item.description || item.filename}
      onerror={() => {
        void onerror();
      }}
    />
  {:else}
    <img
      src={url}
      alt={item.description || item.filename}
      class="h-full w-full bg-surface object-contain outline-none"
      onerror={() => {
        void onerror();
      }}
    />
  {/if}
{:else if url && type.startsWith('video/')}
  <video
    src={url}
    controls
    playsinline
    preload="metadata"
    class="h-full w-full object-contain"
    onerror={() => {
      void onerror();
    }}><track kind="captions" /></video
  >
{:else if url && type.startsWith('audio/')}
  <audio
    src={url}
    controls
    preload="metadata"
    class="w-full max-w-lg"
    onerror={() => {
      void onerror();
    }}>{item.filename}</audio
  >
{:else if busy}
  <LoadingFog class="h-full min-h-32 w-full" />
{:else}
  <div class="flex flex-col items-center gap-3 p-4 text-center text-muted">
    <span class="iconify icon-[uil--file-alt] text-4xl" aria-hidden="true"></span>
    <p>{m('room.attachment.html_viewer.no_preview')}</p>
  </div>
{/if}
