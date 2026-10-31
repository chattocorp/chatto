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
  import { m } from '$lib/i18n/messages';
  import { PreviewToolbar, PreviewAction } from '$lib/ui';

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

<!-- Context menu only. The same actions stay available from the message action menu. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group/preview relative my-1.5 w-full max-w-md preview-frame"
  data-testid="youtube-embed"
  oncontextmenu={onContextMenu}
>
  <iframe
    src="https://www.youtube-nocookie.com/embed/{videoId}"
    title={m('preview.youtube_title')}
    class="aspect-video w-full preview-content"
    allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
    allowfullscreen
  ></iframe>
  {#if (showDismiss && onDismiss) || onDelete}
    <PreviewToolbar>
      {#if showDismiss && onDismiss}
        <PreviewAction
          label={m('preview.youtube_dismiss')}
          icon="icon-[uil--times]"
          onclick={onDismiss}
        />
      {:else}
        <PreviewAction
          label={m('preview.youtube_delete')}
          icon="icon-[uil--trash-alt]"
          onclick={onDelete}
        />
      {/if}
    </PreviewToolbar>
  {/if}
</div>
