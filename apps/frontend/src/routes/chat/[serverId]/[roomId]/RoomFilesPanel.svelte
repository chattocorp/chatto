<!--
@component

Room-scoped file list for the room sidebar.
-->
<script lang="ts">
  import { pushState } from '$app/navigation';
  import { VideoProcessingStatus } from '$lib/render/messageAttachments';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import type { RoomFileItem, RoomFilesStore } from '$lib/state/room';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { useExpiringAssetUrlRefresh } from '$lib/attachments/useExpiringAssetUrlRefresh.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { fileDateGroup, formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { m } from '$lib/i18n/messages';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';

  type RoomFileListItem = {
    id: string;
    file: RoomFileItem;
  };

  type RoomFileGroup = {
    id: string;
    label: string;
    items: RoomFileListItem[];
  };

  let {
    store,
    serverId,
    roomId,
    fileGroupingNow,
    onOpenFileMessage
  }: {
    store: RoomFilesStore;
    serverId: string;
    roomId: string;
    fileGroupingNow?: Date;
    onOpenFileMessage?: (messageEventId: string, threadRootEventId: string | null) => void;
  } = $props();

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());

  const files = $derived(store.items);
  const fileGroups = $derived.by(() => groupFiles(files));
  const fileSections = $derived(
    fileGroups.map((group) => ({
      ...group,
      persistKey: serverStorageKey(
        serverId,
        `collapsible:room-files:${roomId}:${group.id}`
      ),
      testid: 'room-file-group-heading'
    }))
  );
  const loading = $derived(store.isInitialLoading);
  let failedThumbnailUrls = $state.raw(new Set<string>());

  function groupFiles(items: RoomFileItem[]): RoomFileGroup[] {
    const groups: RoomFileGroup[] = [];

    for (const item of items) {
      const group = fileGroupingNow
        ? fileDateGroup(item.createdAt, userSettings, fileGroupingNow, activeLocale)
        : fileDateGroup(item.createdAt, userSettings, undefined, activeLocale);
      let existing = groups.find((candidate) => candidate.id === group.key);
      if (!existing) {
        existing = { id: group.key, label: group.label, items: [] };
        groups.push(existing);
      }
      existing.items.push({
        id: `${item.messageEventId}:${item.attachment.id}`,
        file: item
      });
    }

    return groups;
  }

  function normalizeUrl(url: string | null | undefined): string | null {
    if (!url) return null;
    return assetUrlForServer(serverId, url) ?? url;
  }

  function thumbnailUrl(item: RoomFileItem): string | null {
    return normalizeUrl(store.thumbnailAssetUrlFor(item)?.url);
  }

  function thumbnailFailed(url: string | null): boolean {
    return !!url && failedThumbnailUrls.has(url);
  }

  function usableThumbnailUrl(url: string | null): string | null {
    return thumbnailFailed(url) ? null : url;
  }

  function fileIcon(contentType: string): string {
    if (contentType.startsWith('image/')) return 'icon-[mdi--file-image-outline]';
    if (contentType.startsWith('video/')) return 'icon-[mdi--file-video-outline]';
    if (contentType.startsWith('audio/')) return 'icon-[mdi--file-music-outline]';
    if (contentType === 'application/pdf') return 'icon-[mdi--file-pdf-box]';
    return 'icon-[mdi--file-outline]';
  }

  function openFile(item: RoomFileItem): void {
    const processing = item.attachment.videoProcessing;
    pushState('', {
      modal: {
        type: 'attachmentViewer',
        serverId,
        roomId,
        eventId: item.messageEventId,
        items: [{
          ...item.attachment,
          assetUrl: store.assetUrlFor(item),
          videoProcessing: processing ? {
            ...processing,
            status: {
              PROCESSING: VideoProcessingStatus.Processing,
              COMPLETED: VideoProcessingStatus.Completed,
              FAILED: VideoProcessingStatus.Failed
            }[processing.status]
          } : null
        }],
        index: 0
      }
    });
  }

  function handleThumbnailError(item: RoomFileItem, url: string): void {
    failedThumbnailUrls = new Set([...failedThumbnailUrls, url]);
    void store.refreshUrlsForItem(item);
  }

  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () => store.hasMore ? store.items.length : null,
    loadMore: () => store.loadMore()
  });

  function formatTimestamp(value: string): string {
    return formatDateTime(value, userSettings, activeLocale);
  }

  useExpiringAssetUrlRefresh({
    getRefreshAt: () => store.nextAssetUrlRefreshAt,
    hasStaleUrl: () => store.hasRefreshableStaleUrl(),
    refresh: () => store.refreshStaleUrls(),
    errorMessage: 'Failed to refresh room file URLs',
    refreshOnFocus: false
  });
</script>

{#snippet fileRow(entry: RoomFileListItem)}
  {@const item = entry.file}
  {@const thumb = usableThumbnailUrl(thumbnailUrl(item))}
  <div class="sidebar-item min-h-14 min-w-0 gap-3" data-testid="room-file-row">
    <button
      type="button"
      class="flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center overflow-hidden rounded-md border border-border bg-surface text-muted"
      onclick={() => openFile(item)}
      aria-label={m('room.attachment.view_label', { filename: item.attachment.filename })}
      title={m('room.attachment.view_label', { filename: item.attachment.filename })}
    >
      {#if thumb}
        <img
          class="h-full w-full object-cover"
          src={thumb}
          alt=""
          loading="lazy"
          onerror={() => handleThumbnailError(item, thumb)}
        />
      {:else}
        <span
          class={['iconify sidebar-icon text-xl', fileIcon(item.attachment.contentType)]}
          aria-hidden="true"
        ></span>
      {/if}
    </button>
    <div class="min-w-0 flex-1">
      <div class="flex min-w-0 items-center gap-1.5">
        <button
          type="button"
          class="min-w-0 cursor-pointer text-start text-sm"
          onclick={() => openFile(item)}
          title={m('room.attachment.view_label', { filename: item.attachment.filename })}
          data-testid="room-file-preview"
          aria-describedby={item.attachment.description ? `${entry.id}-description` : undefined}
        >
          <bdi class="block truncate">{item.attachment.filename}</bdi>
        </button>
        {#if onOpenFileMessage}
          <button
            type="button"
            class="inline-flex shrink-0 cursor-pointer text-muted/40 hover:text-muted"
            onclick={() => onOpenFileMessage?.(item.messageEventId, item.threadRootEventId ?? null)}
            aria-label={m('room.sidebar.go_to_message')}
            title={m('room.sidebar.go_to_message')}
            data-testid="room-file-message"
          >
            <span class="iconify icon-[mdi--arrow-right-circle] text-sm rtl:rotate-180" aria-hidden="true"></span>
          </button>
        {/if}
      </div>
      {#if item.attachment.description}
        <span id={`${entry.id}-description`} class="block truncate text-xs text-muted" dir="auto">
          {item.attachment.description}
        </span>
      {/if}
      <span class="block truncate text-xs text-muted">{formatTimestamp(item.createdAt)}</span>
    </div>
  </div>
{/snippet}

<nav
  class="flex min-h-0 flex-1 flex-col overflow-y-auto"
  aria-label={m('room.sidebar.files')}
  aria-busy={loading}
>
  {#if !loading}
    {#if files.length === 0}
      <div
        class="flex min-h-32 flex-1 items-center justify-center px-4 text-center text-sm text-muted"
      >
        {m('room.sidebar.no_files')}
      </div>
    {:else}
      {#each fileSections as section, i (section.id)}
        <RoomGroupSection
          label={section.label}
          items={section.items}
          item={fileRow}
          persistKey={section.persistKey}
          testid={section.testid}
          separated={i > 0}
        />
      {/each}

      {#if store.hasMore}
        <div
          class="flex justify-center px-3 py-4 text-sm text-muted"
          data-testid="room-files-load-more-sentinel"
          {@attach loadMoreWhenVisible}
        >
          {store.isLoadingMore ? m('room.sidebar.loading_files') : ''}
        </div>
      {/if}
    {/if}
  {/if}
</nav>
