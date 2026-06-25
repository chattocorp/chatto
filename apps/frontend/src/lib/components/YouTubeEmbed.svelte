<!--
@component

Renders an embedded YouTube video player using youtube-nocookie.com for privacy.
Supports dismiss (composer) and delete (posted message) actions.
When `canDelete` is true, right-click / long-press opens a context menu with Open on YouTube, Copy URL, and Delete.

**Props:**
- `videoId` - The YouTube video ID to embed
- `url` - The original YouTube URL (for context menu actions)
- `onDismiss` - Callback when user dismisses the embed (composer mode)
- `showDismiss` - Whether to show the dismiss button (default: true)
- `canDelete` - Whether the user can delete this embed (default: false)
- `roomId` - Room ID (required when canDelete is true, for confirmation dialog)
- `eventId` - Message body ID (required when canDelete is true, for confirmation dialog)
-->
<script lang="ts">
  import { pushState } from '$app/navigation';
  import * as m from '$lib/i18n/messages';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { toast } from '$lib/ui/toast';

  let {
    videoId,
    url,
    onDismiss,
    showDismiss = true,
    canDelete = false,
    roomId,
    eventId
  }: {
    videoId: string;
    url?: string;
    onDismiss?: () => void;
    showDismiss?: boolean;
    canDelete?: boolean;
    roomId?: string;
    eventId?: string;
  } = $props();

  const youtubeUrl = $derived(url ?? `https://www.youtube.com/watch?v=${videoId}`);

  // Context menu state
  let contextMenuPos = $state<{ x: number; y: number } | null>(null);

  function openDeleteConfirmation() {
    if (!roomId || !eventId) return;
    pushState('', {
      modal: {
        type: 'deleteLinkPreview',
        roomId,
        eventId,
        previewUrl: youtubeUrl
      }
    });
  }

  function handleContextMenu(e: MouseEvent) {
    if (!canDelete) return;
    e.preventDefault();
    e.stopPropagation();
    contextMenuPos = { x: e.clientX, y: e.clientY };
  }

  async function handleCopyUrl() {
    try {
      await navigator.clipboard.writeText(youtubeUrl);
      toast.success('URL copied to clipboard');
    } catch {
      toast.error('Failed to copy URL');
    }
    contextMenuPos = null;
  }

  function handleOpenLink() {
    window.open(youtubeUrl, '_blank', 'noopener,noreferrer');
    contextMenuPos = null;
  }

  function handleDeleteFromMenu() {
    openDeleteConfirmation();
    contextMenuPos = null;
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group/preview relative embed-frame w-full max-w-md"
  data-testid="youtube-embed"
  oncontextmenu={handleContextMenu}
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
  {:else if canDelete}
    <button
      type="button"
      onclick={openDeleteConfirmation}
      class="embed-control-button md:group-hover/preview:opacity-100"
      aria-label={m['preview.youtube_delete']()}
    >
      <span class="iconify text-sm uil--times"></span>
    </button>
  {/if}
</div>

<!-- Context menu (posted message mode only) -->
{#if contextMenuPos}
  <ContextMenu position={contextMenuPos} onclose={() => (contextMenuPos = null)}>
    <div class="menu-section">
      <nav class="sidebar-nav">
        <button class="sidebar-item" onclick={handleOpenLink} role="menuitem">
          <span class="sidebar-icon iconify uil--external-link-alt"></span>
          {m['preview.youtube_open']()}
        </button>
        <button class="sidebar-item" onclick={handleCopyUrl} role="menuitem">
          <span class="sidebar-icon iconify uil--copy"></span>
          {m['preview.copy_url']()}
        </button>
        {#if canDelete}
          <button
            class="sidebar-item text-danger hover:text-danger"
            onclick={handleDeleteFromMenu}
            role="menuitem"
          >
            <span class="sidebar-icon iconify uil--trash-alt"></span>
            {m['preview.youtube_delete_embed']()}
          </button>
        {/if}
      </nav>
    </div>
  </ContextMenu>
{/if}
