<!--
@component

Room-scoped file list for the room sidebar.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { RoomFileItem, RoomFilesStore } from '$lib/state/room';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { fileDateGroup, formatDateTime } from '$lib/utils/formatTime';

  type RoomFileGroup = {
    key: string;
    label: string;
    items: RoomFileItem[];
  };

  let {
    store,
    serverId,
    fileGroupingNow,
    onOpenFile
  }: {
    store: RoomFilesStore;
    serverId: string;
    fileGroupingNow?: Date;
    onOpenFile?: (messageEventId: string, threadRootEventId: string | null) => void;
  } = $props();

  const userSettings = getUserSettings();

  const files = $derived(store.items);
  const fileGroups = $derived.by(() => groupFiles(files));
  const loading = $derived(store.isInitialLoading);
  let failedThumbnailUrls = $state.raw(new Set<string>());

  function groupFiles(items: RoomFileItem[]): RoomFileGroup[] {
    const groups: RoomFileGroup[] = [];

    for (const item of items) {
      const group = fileGroupingNow
        ? fileDateGroup(item.createdAt, userSettings, fileGroupingNow)
        : fileDateGroup(item.createdAt, userSettings);
      let existing = groups.find((candidate) => candidate.key === group.key);
      if (!existing) {
        existing = { ...group, items: [] };
        groups.push(existing);
      }
      existing.items.push(item);
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
    if (contentType.startsWith('image/')) return 'mdi--file-image-outline';
    if (contentType.startsWith('video/')) return 'mdi--file-video-outline';
    if (contentType.startsWith('audio/')) return 'mdi--file-music-outline';
    if (contentType === 'application/pdf') return 'mdi--file-pdf-box';
    return 'mdi--file-outline';
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
    return formatDateTime(value, userSettings);
  }

  $effect(() => {
    const refreshAt = store.nextAssetUrlRefreshAt;
    if (refreshAt === null) return;

    const timeout = window.setTimeout(
      () => {
        store.refreshStaleUrls().catch((error: unknown) => {
          console.warn('Failed to refresh room file URLs before expiry', error);
        });
      },
      Math.max(0, refreshAt - Date.now())
    );

    return () => window.clearTimeout(timeout);
  });

  function handleVisibilityChange(): void {
    if (document.visibilityState !== 'visible') return;
    store.refreshStaleUrls().catch((error: unknown) => {
      console.warn('Failed to refresh stale room file URLs', error);
    });
  }

  onMount(() => {
    void store.refreshStaleUrls();
  });
</script>

<svelte:document onvisibilitychange={handleVisibilityChange} />

<nav class="flex min-h-0 flex-1 flex-col overflow-y-auto p-2" aria-label="Files">
  {#if loading}
    <ul role="list" class="space-y-1">
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
      No files in this room yet.
    </div>
  {:else}
    <div class="space-y-4">
      {#each fileGroups as group (group.key)}
        <section aria-labelledby={`room-file-group-${group.key}`}>
          <h2
            id={`room-file-group-${group.key}`}
            class="px-2 pb-1 text-xs font-medium tracking-wide text-muted uppercase"
            data-testid="room-file-group-heading"
          >
            {group.label}
          </h2>
          <ul role="list" class="space-y-1">
            {#each group.items as item (item.messageEventId + ':' + item.attachment.id)}
              {@const thumb = usableThumbnailUrl(thumbnailUrl(item))}
              <li>
                <button
                  type="button"
                  class="sidebar-item min-h-14 w-full cursor-pointer gap-3 text-left"
                  onclick={() => openFile(item)}
                  title={`Jump to ${item.attachment.filename}`}
                  data-testid="room-file-row"
                >
                  <span
                    class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-md border border-border bg-surface-100 text-muted"
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
                        class={[
                          'sidebar-icon iconify text-xl',
                          fileIcon(item.attachment.contentType)
                        ]}
                        aria-hidden="true"
                      ></span>
                    {/if}
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm">{item.attachment.filename}</span>
                    <span class="block truncate text-xs text-muted"
                      >{formatTimestamp(item.createdAt)}</span
                    >
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>

    {#if store.hasMore}
      <div
        class="flex justify-center px-3 py-4 text-sm text-muted"
        data-testid="room-files-load-more-sentinel"
        {@attach loadMoreWhenVisible}
      >
        {store.isLoadingMore ? 'Loading files...' : ''}
      </div>
    {/if}
  {/if}
</nav>
