<!--
@component

Displays a compact, one-row link preview in the message composer. The composer
must stay short so the text field stays visible above a software keyboard,
so this card shows only a thumbnail, the site, and the title. Posted
messages use the full `LinkPreviewCard`. YouTube and social-post previews also
use this row, so the composer does not load a third-party player while the
user types.

**Props:**
- `preview` - The LinkPreview data to display
- `onDismiss` - Callback when the user dismisses the preview
-->
<script lang="ts">
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { m } from '$lib/i18n/messages';
  import { renderedLinkTarget, handleRenderedLinkClick } from '$lib/navigation/renderedLink';

  let { preview, onDismiss }: { preview: LinkPreviewView; onDismiss: () => void } = $props();

  const hostname = $derived.by(() => {
    try {
      return new URL(preview.url).hostname;
    } catch {
      return preview.url;
    }
  });
  const thumbnailUrl = $derived(preview.imageUrl || preview.socialPost?.images[0]?.url || null);
  const title = $derived(
    preview.title || preview.socialPost?.author?.displayName || preview.socialPost?.text || hostname
  );
  let failedThumbnailUrl = $state<string | null>(null);
</script>

<div
  data-testid="composer-link-preview"
  class="flex w-full max-w-md min-w-0 items-center gap-2 rounded-md bg-surface p-2"
>
  <a
    {...renderedLinkTarget(preview.url)}
    onclick={(event) => handleRenderedLinkClick(event, preview.url)}
    onauxclick={(event) => handleRenderedLinkClick(event, preview.url)}
    class="flex min-w-0 flex-1 items-center gap-2"
  >
    <div
      class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-surface-emphasized"
    >
      {#if thumbnailUrl && failedThumbnailUrl !== thumbnailUrl}
        <img
          src={thumbnailUrl}
          alt=""
          class="h-full w-full object-cover"
          onerror={() => (failedThumbnailUrl = thumbnailUrl)}
        />
      {:else}
        <span
          class={[
            'iconify text-lg text-muted',
            preview.embedType === 'youtube' ? 'icon-[uil--play]' : 'icon-[uil--link]'
          ]}
          aria-hidden="true"
        ></span>
      {/if}
    </div>
    <div class="flex min-w-0 flex-1 flex-col">
      <span class="truncate text-xs text-muted">{preview.siteName || hostname}</span>
      <span class="truncate text-sm font-medium text-text">{title}</span>
    </div>
  </a>
  <button
    type="button"
    onclick={onDismiss}
    class="mini-icon-action h-8 w-8 items-center justify-center hover:text-danger"
    aria-label={m('preview.dismiss')}
    title={m('preview.dismiss')}
  >
    <span class="iconify icon-[uil--times] text-base" aria-hidden="true"></span>
  </button>
</div>
