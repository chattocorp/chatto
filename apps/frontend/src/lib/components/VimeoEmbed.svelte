<!--
@component

Shows the Vimeo player with its Do Not Track option. Loading the player connects the
browser to Vimeo. The parent supplies a URL from `vimeoEmbedUrl` and owns preview actions.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { PreviewToolbar, PreviewAction } from '$lib/ui';

  let {
    embedUrl,
    title,
    onDismiss,
    showDismiss = true,
    onContextMenu,
    onDelete
  }: {
    embedUrl: string;
    title?: string | null;
    onDismiss?: () => void;
    showDismiss?: boolean;
    onContextMenu?: (event: MouseEvent) => void;
    onDelete?: () => void;
  } = $props();
</script>

<!-- The same context-menu actions are available from the message action menu. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group/preview relative my-1.5 w-full max-w-md preview-frame"
  data-testid="vimeo-embed"
  oncontextmenu={onContextMenu}
>
  <iframe
    src={embedUrl}
    title={title || 'Vimeo'}
    class="block aspect-video w-full preview-content"
    loading="lazy"
    allow="fullscreen; picture-in-picture; encrypted-media"
    allowfullscreen
  ></iframe>
  {#if (showDismiss && onDismiss) || onDelete}
    <PreviewToolbar>
      {#if showDismiss && onDismiss}
        <PreviewAction label={m('preview.dismiss')} icon="icon-[uil--times]" onclick={onDismiss} />
      {:else}
        <PreviewAction
          label={m('preview.delete')}
          icon="icon-[uil--trash-alt]"
          onclick={onDelete}
        />
      {/if}
    </PreviewToolbar>
  {/if}
</div>
