<!--
@component

Displays a compact, one-row link preview in the message composer. The composer
must stay short so the text field stays visible above a software keyboard,
so this card shows only a thumbnail, the site, and the title. Posted
messages use the full `LinkPreviewCard`. YouTube and social-post previews also
use this row, so the composer does not load a third-party player while the
user types. A social post with a content warning shows no image or post text.

**Props:**
- `preview` - The LinkPreview data to display
- `onDismiss` - Callback when the user dismisses the preview
-->
<script lang="ts">
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { m } from '$lib/i18n/messages';
  import { renderedLinkTarget, handleRenderedLinkClick } from '$lib/navigation/renderedLink';

  let { preview, onDismiss }: { preview: LinkPreviewView; onDismiss: () => void } = $props();

  /** Host and path without the scheme, for previews that have no site name or title. */
  const urlParts = $derived.by(() => {
    try {
      const url = new URL(preview.url);
      const path = decodeURI(url.pathname).replace(/\/$/, '');
      return { host: url.hostname, label: `${url.hostname}${path}` };
    } catch {
      return { host: preview.url, label: preview.url };
    }
  });
  /** A content warning on the post or its quoted post hides its media and text here too. */
  const concealed = $derived(
    Boolean(preview.socialPost?.contentWarning || preview.socialPost?.quotedPost?.contentWarning)
  );
  const thumbnailUrl = $derived(concealed ? null : preview.imageUrl || null);
  const title = $derived(
    preview.title ||
      preview.socialPost?.author?.displayName ||
      (concealed ? null : preview.socialPost?.text) ||
      urlParts.label
  );
  let failedThumbnailUrl = $state<string | null>(null);
</script>

<div data-testid="composer-link-preview" class="w-full max-w-md min-w-0 preview-frame">
  <div class="flex min-w-0 items-center gap-2 preview-content px-2 py-1.5">
    <a
      {...renderedLinkTarget(preview.url)}
      onclick={(event) => handleRenderedLinkClick(event, preview.url)}
      class="flex min-w-0 flex-1 items-center gap-2"
    >
      <div class="flex h-12 w-12 shrink-0 items-center justify-center preview-thumbnail">
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
              concealed
                ? 'icon-[uil--eye-slash]'
                : preview.embedType === 'youtube'
                  ? 'icon-[uil--play]'
                  : 'icon-[uil--link]'
            ]}
            aria-hidden="true"
          ></span>
        {/if}
      </div>
      <div class="flex min-w-0 flex-1 flex-col">
        <span class="truncate text-sm text-muted" dir="auto"
          >{preview.siteName || urlParts.host}</span
        >
        <span class="truncate text-sm font-medium text-text" dir="auto">{title}</span>
      </div>
    </a>
    <button
      type="button"
      onclick={onDismiss}
      class="mini-icon-action h-8 w-8 items-center justify-center hover:text-danger comfortable-density:h-11 comfortable-density:w-11"
      aria-label={m('preview.dismiss')}
      title={m('preview.dismiss')}
    >
      <span class="iconify icon-[uil--times] text-base" aria-hidden="true"></span>
    </button>
  </div>
</div>
