<!--
@component

Room-scoped file list for the room sidebar.
-->
<script lang="ts">
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
    onOpenFile
  }: {
    store: RoomFilesStore;
    serverId: string;
    roomId: string;
    fileGroupingNow?: Date;
    onOpenFile?: (messageEventId: string, threadRootEventId: string | null) => void;
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
    onOpenFile?.(item.messageEventId, item.threadRootEventId ?? null);
  }

  function handleThumbnailError(item: RoomFileItem, url: string): void {
    failedThumbnailUrls = new Set([...failedThumbnailUrls, url]);
    void store.refreshUrlsForItem(item);
  }

  function loadMoreWhenVisible(node: HTMLElement) {
    if (typeof IntersectionObserver === 'undefined') return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        if (!store.hasMore || store.isLoadingMore) return;
        void store.loadMore();
      },
      { rootMargin: '160px 0px' }
    );
    observer.observe(node);

    return () => observer.disconnect();
  }

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
  <button
    type="button"
    class="sidebar-item min-h-14 w-full cursor-pointer gap-3 text-start"
    onclick={() => openFile(item)}
    title={m('room.sidebar.jump_to_file', { filename: item.attachment.filename })}
    data-testid="room-file-row"
  >
    <span
      class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-md border border-border bg-surface text-muted"
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
    </span>
    <span class="min-w-0 flex-1">
      <bdi class="block truncate text-sm">{item.attachment.filename}</bdi>
      <span class="block truncate text-xs text-muted">{formatTimestamp(item.createdAt)}</span>
    </span>
  </button>
{/snippet}

<nav class="flex min-h-0 flex-1 flex-col overflow-y-auto" aria-label={m('room.sidebar.files')}>
  {#if loading}
    <ul role="list" class="space-y-1 p-2">
      {#each Array(8) as _, i (i)}
        <li class="flex items-center gap-3 rounded-md px-2 py-2">
          <div class="skeleton h-10 w-10 shrink-0 rounded-md"></div>
          <div class="min-w-0 flex-1 space-y-1">
            <div class="skeleton h-3.5 w-32 rounded"></div>
            <div class="skeleton h-3 w-24 rounded"></div>
          </div>
        </li>
      {/each}
    </ul>
  {:else if files.length === 0}
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
</nav>
