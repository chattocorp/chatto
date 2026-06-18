<!--
@component

Displays an OpenGraph link preview card with image, title, description, and site name.
YouTube URLs render a YouTubeEmbed instead. Supports dismiss (composer) and delete (posted message) actions.
When `canDelete` is true, right-click / long-press opens a context menu with Open link, Copy URL, and Delete.

**Props:**
- `preview` - The LinkPreview data to display
- `onDismiss` - Callback when user dismisses the preview (composer mode)
- `showDismiss` - Whether to show the dismiss button (default: true)
- `canDelete` - Whether the user can delete this preview (default: false)
- `roomId` - Room ID (required when canDelete is true, for confirmation dialog)
- `eventId` - Message body ID (required when canDelete is true, for confirmation dialog)
-->
<script lang="ts" module>
  import { graphql } from '$lib/gql';

  export const LinkPreviewFragment = graphql(`
    fragment LinkPreviewView on LinkPreview {
      url
      title
      description
      imageUrl(width: 600, height: 314, fit: CONTAIN)
      siteName
      embedType
      embedId
    }
  `);
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- external URL from link preview */
  import type { FragmentType } from '$lib/gql/fragment-masking';
  import { useFragment } from '$lib/gql/fragment-masking';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { pushState } from '$app/navigation';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { toast } from '$lib/ui/toast';
  import YouTubeEmbed from './YouTubeEmbed.svelte';

  let {
    preview: rawPreview,
    onDismiss,
    showDismiss = true,
    canDelete = false,
    roomId,
    eventId
  }: {
    preview: FragmentType<typeof LinkPreviewFragment>;
    onDismiss?: () => void;
    showDismiss?: boolean;
    canDelete?: boolean;
    roomId?: string;
    eventId?: string;
  } = $props();

  const preview = $derived(useFragment(LinkPreviewFragment, rawPreview));

  // Context menu state
  let contextMenuPos = $state<{ x: number; y: number } | null>(null);

  function openDeleteConfirmation() {
    if (!roomId || !eventId) return;
    pushState('', {
      modal: {
        type: 'deleteLinkPreview',
        roomId,
        eventId,
        previewUrl: preview.url
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
      await navigator.clipboard.writeText(preview.url);
      toast.success('URL copied to clipboard');
    } catch {
      toast.error('Failed to copy URL');
    }
    contextMenuPos = null;
  }

  function handleOpenLink() {
    window.open(preview.url, '_blank', 'noopener,noreferrer');
    contextMenuPos = null;
  }

  function handleDeleteFromMenu() {
    openDeleteConfirmation();
    contextMenuPos = null;
  }
</script>

{#if preview.embedType === 'youtube' && preview.embedId}
  <YouTubeEmbed
    videoId={preview.embedId}
    url={preview.url}
    {onDismiss}
    {showDismiss}
    {canDelete}
    {roomId}
    {eventId}
  />
{:else if preview.imageUrl || preview.title || preview.description || preview.siteName}
  <a
    href={preview.url}
    target="_blank"
    rel="noopener noreferrer"
    data-testid="link-preview-card"
    class="group/preview relative flex w-full max-w-md flex-col embed-frame"
    oncontextmenu={handleContextMenu}
  >
    {#if preview.imageUrl}
      <SkeletonImg
        src={preview.imageUrl}
        alt=""
        class="aspect-[1.91/1] w-full rounded-sm object-cover"
        onerror={(e) => {
          // Hide the image if it fails to load
          (e.target as HTMLImageElement).style.display = 'none';
        }}
      />
    {/if}
    <div class="flex min-w-0 flex-col gap-0.5 px-3 pt-3 pb-2">
      {#if preview.siteName}
        <span class="text-xs tracking-wide text-muted uppercase">{preview.siteName}</span>
      {/if}
      {#if preview.title}
        <span class="line-clamp-2 text-sm leading-snug font-medium">{preview.title}</span>
      {/if}
      {#if preview.description}
        <span class="line-clamp-2 text-xs text-muted">{preview.description}</span>
      {/if}
    </div>
    {#if showDismiss && onDismiss}
      <button
        type="button"
        onclick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onDismiss?.();
        }}
        class="embed-control-button md:group-hover/preview:opacity-100"
        aria-label="Dismiss preview"
      >
        <span class="iconify text-sm uil--times"></span>
      </button>
    {:else if canDelete}
      <button
        type="button"
        onclick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          openDeleteConfirmation();
        }}
        class="embed-control-button md:group-hover/preview:opacity-100"
        aria-label="Delete preview"
      >
        <span class="iconify text-sm uil--times"></span>
      </button>
    {/if}
  </a>

  <!-- Context menu (posted message mode only) -->
  {#if contextMenuPos}
    <ContextMenu position={contextMenuPos} onclose={() => (contextMenuPos = null)}>
      <div class="menu-section">
        <nav class="sidebar-nav">
          <button class="sidebar-item" onclick={handleOpenLink} role="menuitem">
            <span class="sidebar-icon iconify uil--external-link-alt"></span>
            Open link
          </button>
          <button class="sidebar-item" onclick={handleCopyUrl} role="menuitem">
            <span class="sidebar-icon iconify uil--copy"></span>
            Copy URL
          </button>
          {#if canDelete}
            <button
              class="sidebar-item text-danger hover:text-danger"
              onclick={handleDeleteFromMenu}
              role="menuitem"
            >
              <span class="sidebar-icon iconify uil--trash-alt"></span>
              Delete preview
            </button>
          {/if}
        </nav>
      </div>
    </ContextMenu>
  {/if}
{/if}
