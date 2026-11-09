<script lang="ts">
  import PreviewCard from './ui/PreviewCard.svelte';
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { mediaUrl, openMedia } from './media.ts';
  import NativeImage from './NativeImage.svelte';
  import SocialPreview from './SocialPreview.svelte';
  import ActionButton from './ActionButton.svelte';

  let {
    preview,
    serverUrl,
    active
  }: { preview: LinkPreviewView; serverUrl: string; active: boolean } = $props();
  const url = $derived(mediaUrl(preview.url, serverUrl));
  let error = $state('');
  async function open() {
    if (!url) return;
    try {
      await openMedia(url);
    } catch {
      error = 'Could not open link.';
    }
  }
</script>

<PreviewCard testid="link-preview">
  {#if preview.socialPost}
    <SocialPreview post={preview.socialPost} {serverUrl} {active} />
  {:else}
    {#if preview.imageUrl}<NativeImage
        src={preview.imageUrl}
        {serverUrl}
        {active}
        alt={preview.title || 'Link preview'}
      />{/if}
    <text class="site">{preview.siteName || url?.host || 'Link preview'}</text>
    {#if preview.title}<text class="title">{preview.title}</text>{/if}
    {#if preview.description}<text class="description">{preview.description}</text>{/if}
    {#if preview.embedType}<text class="site">{preview.embedType} · opens in your browser</text
      >{/if}
  {/if}
  <ActionButton quiet label="Open link in browser" onclick={() => void open()} disabled={!url} />
  {#if error}<text class="error">{error}</text>{/if}
</PreviewCard>

<style>
  .site {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .title {
    font-size: var(--text-body);
    font-weight: bold;
  }
  .description {
    font-size: var(--text-body);
    white-space: pre-wrap;
    user-select: text;
  }
  .error {
    font-size: var(--text-small);
    color: var(--color-error);
  }
</style>
