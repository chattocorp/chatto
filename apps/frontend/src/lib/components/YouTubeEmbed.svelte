<!--
@component

Renders an embedded YouTube video player using youtube-nocookie.com for privacy.
Its parent owns shared link-preview actions and passes the relevant callbacks.

**Props:**
- `videoId` - The YouTube video ID to embed
- `onDismiss` - Callback when user dismisses the embed (composer mode)
- `showDismiss` - Whether to show the dismiss button (default: true)
- `onContextMenu` - Callback for a permitted posted-message context menu
- `onDelete` - Callback for deleting the posted embed
-->
<script lang="ts">
  import * as m from '$lib/i18n/messages';

  let {
    videoId,
    onDismiss,
    showDismiss = true,
    onContextMenu,
    onDelete
  }: {
    videoId: string;
    onDismiss?: () => void;
    showDismiss?: boolean;
    onContextMenu?: (event: MouseEvent) => void;
    onDelete?: () => void;
  } = $props();
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group/preview relative embed-frame w-full max-w-md"
  data-testid="youtube-embed"
  oncontextmenu={onContextMenu}
>
  <iframe
    src="https://www.youtube-nocookie.com/embed/{videoId}"
    title={m['preview.youtube_title']()}
    class="aspect-video w-full"
    allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
    allowfullscreen
  ></iframe>
  {#if showDismiss && onDismiss}
    <button
      type="button"
      onclick={onDismiss}
      class="embed-control-button md:group-hover/preview:opacity-100"
      aria-label={m['preview.youtube_dismiss']()}
    >
      <span class="iconify text-sm uil--times"></span>
    </button>
  {:else if onDelete}
    <button
      type="button"
      onclick={onDelete}
      class="embed-control-button md:group-hover/preview:opacity-100"
      aria-label={m['preview.youtube_delete']()}
    >
      <span class="iconify text-sm uil--times"></span>
    </button>
  {/if}
</div>
