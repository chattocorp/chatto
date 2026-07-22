import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { FitMode } from '$lib/render/types';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import {
  assetUrlNeedsRefresh,
  earliestAssetUrlRefreshAt,
  mergeRefreshedAttachmentUrls,
  refreshAttachmentUrlsForAssets
} from '$lib/attachments/attachmentUrls';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { RoomTimelineEvent } from '@chatto/api-types/api/v1/room_timeline_pb';
import {
  createAttachmentAPI,
  roomFileItemsForTimelineEvent,
  type AttachmentAPI,
  type RoomFileItem
} from '$lib/api-client/attachments';

export const ROOM_FILES_PAGE_SIZE = 50;

export type { RoomFileItem };

function itemKey(item: RoomFileItem): string {
  return `${item.messageEventId}:${item.attachment.id}`;
}

function attachmentState(item: RoomFileItem) {
  const attachment = item.attachment;
  const processing = attachment.videoProcessing;
  return {
    messageEventId: item.messageEventId,
    threadRootEventId: item.threadRootEventId,
    createdAt: item.createdAt,
    id: attachment.id,
    filename: attachment.filename,
    contentType: attachment.contentType,
    width: attachment.width,
    height: attachment.height,
    processing: processing
      ? {
          status: processing.status,
          durationMs: processing.durationMs,
          width: processing.width,
          height: processing.height,
          sourceAvailable: processing.sourceAvailable,
          reasonCode: processing.reasonCode,
          hasHLS: Boolean(processing.hlsMasterPlaylistUrl),
          variants: processing.variants.map(({ quality, width, height, size }) => ({
            quality,
            width,
            height,
            size
          }))
        }
      : null
  };
}

function sameAttachmentState(current: RoomFileItem[], replacement: RoomFileItem[]): boolean {
  return (
    current.length === replacement.length &&
    current.every(
      (item, index) =>
        JSON.stringify(attachmentState(item)) ===
        JSON.stringify(attachmentState(replacement[index]))
    )
  );
}

function attachmentAssetUrls(item: RoomFileItem, refreshed: RefreshedAttachmentUrls | undefined) {
  if (refreshed) {
    return [
      refreshed.assetUrl,
      refreshed.thumbnailAssetUrl,
      refreshed.videoThumbnailAssetUrl,
      refreshed.hlsMasterPlaylistUrl
    ];
  }

  return [
    item.attachment.assetUrl,
    item.attachment.thumbnailAssetUrl,
    item.attachment.videoProcessing?.thumbnailAssetUrl,
    item.attachment.videoProcessing?.hlsMasterPlaylistUrl
  ];
}

function isImageAttachment(contentType: string): boolean {
  return contentType.startsWith('image/');
}

function isVideoAttachment(contentType: string): boolean {
  return contentType.startsWith('video/');
}

export class RoomFilesStore {
  items = $state.raw<RoomFileItem[]>([]);
  totalCount = $state(0);
  hasMore = $state(false);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  refreshedAttachmentUrls = new SvelteMap<string, RefreshedAttachmentUrls>();

  private readonly attachmentAPI: AttachmentAPI;
  private readonly roomId: string;
  private hydrated = false;
  private retainCount = 0;
  private requestEpoch = 0;
  private paginationEpoch = 0;
  private hydrationPromise: Promise<void> | null = null;
  private pendingTimelineEvents: Array<{
    event: RoomTimelineEvent;
    sourceEventId: string;
  }> = [];
  private attachmentVersions = new SvelteMap<string, number>();
  private urlRefreshPromise: Promise<void> | null = null;
  private pendingUrlRefreshAssetIds = new SvelteSet<string>();

  constructor(serverConnection: ServerConnection, roomId: string) {
    this.roomId = roomId;
    this.attachmentAPI = createAttachmentAPI({
      serverId: serverConnection.serverId,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    });
  }

  /** Hydrate this room's file cache the first time its Files panel opens. */
  async hydrate(): Promise<void> {
    await this.ensureFresh();
  }

  /** Keep this cache hydrated while its room Files panel is visible. */
  retain(): () => void {
    this.retainCount++;
    if (this.retainCount === 1) void this.hydrate();

    let retained = true;
    return () => {
      if (!retained) return;
      retained = false;
      this.retainCount = Math.max(0, this.retainCount - 1);
    };
  }

  /** Reconcile a current timeline message into an already-hydrated file cache. */
  applyTimelineEvent(event: RoomTimelineEvent, sourceEventId: string): void {
    const replacement = roomFileItemsForTimelineEvent(event);
    const isNewMessage = event.id === sourceEventId;
    if (isNewMessage && replacement.length === 0) return;

    if (!this.hydrated) {
      if (this.hydrationPromise) {
        this.pendingTimelineEvents = [
          ...this.pendingTimelineEvents.filter((pending) => pending.event.id !== event.id),
          { event, sourceEventId }
        ];
      }
      return;
    }

    const current = this.items.filter((item) => item.messageEventId === event.id);
    if (sameAttachmentState(current, replacement)) return;
    if (!isNewMessage && current.length === 0 && replacement.length === 0 && !this.hasMore) return;

    const retryPagination = this.fencePagination();
    for (const item of [...current, ...replacement]) {
      const attachmentId = item.attachment.id;
      this.attachmentVersions.set(
        attachmentId,
        (this.attachmentVersions.get(attachmentId) ?? 0) + 1
      );
      this.refreshedAttachmentUrls.delete(attachmentId);
    }
    if (!isNewMessage && current.length === 0 && this.hasMore) {
      if (retryPagination) void this.loadMore();
      return;
    }
    if (current.length === 0 && replacement.length === 0) {
      if (retryPagination) void this.loadMore();
      return;
    }

    this.items = [
      ...this.items.filter((item) => item.messageEventId !== event.id),
      ...replacement
    ].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    this.totalCount = Math.max(0, this.totalCount - current.length + replacement.length);
    this.hasMore = this.totalCount > this.items.length;
    if (retryPagination) void this.loadMore();
  }

  /** Clear cached room content, optionally restoring a still-visible panel. */
  reset(options: { rehydrateRetained?: boolean } = {}): void {
    this.fenceRequests();
    this.items = [];
    this.totalCount = 0;
    this.hasMore = false;
    this.isInitialLoading = true;
    this.refreshedAttachmentUrls = new SvelteMap();
    this.attachmentVersions.clear();
    this.hydrated = false;
    this.pendingTimelineEvents = [];
    if (options.rehydrateRetained && this.retainCount > 0) void this.hydrate();
  }

  /** Rehydrate a visible cache after an explicit positive room-access grant. */
  restoreAfterAccessGrant(): void {
    if (this.retainCount > 0 && !this.hydrated) void this.hydrate();
  }

  async loadMore(): Promise<void> {
    if (
      this.hydrationPromise ||
      this.isLoadingMore ||
      !this.hasMore ||
      !this.hydrated
    )
      return;
    const roomId = this.roomId;
    const requestEpoch = this.requestEpoch;
    const paginationEpoch = this.paginationEpoch;
    this.isLoadingMore = true;
    try {
      await this.loadPage(this.items.length, false, ROOM_FILES_PAGE_SIZE, paginationEpoch);
    } finally {
      if (
        this.roomId === roomId &&
        this.requestEpoch === requestEpoch &&
        this.paginationEpoch === paginationEpoch
      ) {
        this.isLoadingMore = false;
      }
    }
  }

  assetUrlFor(item: RoomFileItem): ExpiringAssetUrl | null {
    const refreshed = this.refreshedAttachmentUrls.get(item.attachment.id);
    return refreshed ? refreshed.assetUrl : item.attachment.assetUrl;
  }

  thumbnailAssetUrlFor(item: RoomFileItem): ExpiringAssetUrl | null {
    const refreshed = this.refreshedAttachmentUrls.get(item.attachment.id);
    const contentType = item.attachment.contentType;
    if (isVideoAttachment(contentType)) {
      return refreshed
        ? refreshed.videoThumbnailAssetUrl
        : (item.attachment.videoProcessing?.thumbnailAssetUrl ?? null);
    }
    if (!isImageAttachment(contentType)) return null;

    if (refreshed) {
      return refreshed.thumbnailAssetUrl ?? refreshed.videoThumbnailAssetUrl ?? null;
    }

    return (
      item.attachment.thumbnailAssetUrl ??
      item.attachment.videoProcessing?.thumbnailAssetUrl ??
      null
    );
  }

  get nextAssetUrlRefreshAt(): number | null {
    return earliestAssetUrlRefreshAt(
      this.items.flatMap((item) =>
        attachmentAssetUrls(item, this.refreshedAttachmentUrls.get(item.attachment.id))
      )
    );
  }

  hasRefreshableStaleUrl(): boolean {
    return this.items.some((item) =>
      attachmentAssetUrls(item, this.refreshedAttachmentUrls.get(item.attachment.id)).some((url) =>
        assetUrlNeedsRefresh(url)
      )
    );
  }

  async refreshStaleUrls(): Promise<void> {
    if (!this.hasRefreshableStaleUrl()) return;
    await this.refreshUrlsForItems(this.items);
  }

  async refreshUrlsForItem(item: RoomFileItem): Promise<void> {
    await this.refreshUrlsForItems([item]);
  }

  private async refreshUrlsForItems(items: RoomFileItem[]): Promise<void> {
    if (!this.hydrated || items.length === 0) return;
    for (const item of items) this.pendingUrlRefreshAssetIds.add(item.attachment.id);
    if (this.urlRefreshPromise) return this.urlRefreshPromise;

    const roomId = this.roomId;
    const requestEpoch = this.requestEpoch;
    const refresh = (async () => {
      while (
        this.roomId === roomId &&
        this.requestEpoch === requestEpoch &&
        this.pendingUrlRefreshAssetIds.size > 0
      ) {
        const assetIds = Array.from(this.pendingUrlRefreshAssetIds);
        const requestedVersions = Object.fromEntries(
          assetIds.map((assetId) => [assetId, this.attachmentVersions.get(assetId) ?? 0])
        );
        const freshMap = await refreshAttachmentUrlsForAssets(
          this.attachmentAPI,
          roomId,
          assetIds,
          {
            width: 120,
            height: 120,
            fit: FitMode.Cover
          }
        );
        if (this.roomId !== roomId || this.requestEpoch !== requestEpoch) return;
        for (const assetId of assetIds) this.pendingUrlRefreshAssetIds.delete(assetId);

        const fresh = new SvelteMap<string, RefreshedAttachmentUrls>();
        const currentAssetIds = new SvelteSet(
          this.items.map((item) => item.attachment.id)
        );
        for (const [attachmentId, urls] of freshMap) {
          if (
            !currentAssetIds.has(attachmentId) ||
            (this.attachmentVersions.get(attachmentId) ?? 0) !== requestedVersions[attachmentId]
          )
            continue;
          fresh.set(attachmentId, urls);
        }
        this.refreshedAttachmentUrls = new SvelteMap(
          mergeRefreshedAttachmentUrls(this.refreshedAttachmentUrls, fresh)
        );
      }
    })();
    this.urlRefreshPromise = refresh;
    try {
      await refresh;
    } finally {
      if (this.urlRefreshPromise === refresh) this.urlRefreshPromise = null;
    }
  }

  private async ensureFresh(): Promise<void> {
    if (this.hydrated) return;
    if (this.hydrationPromise) return this.hydrationPromise;

    const hydration = (async () => {
      this.isInitialLoading = true;
      const loaded = await this.loadPage(0, true, ROOM_FILES_PAGE_SIZE);
      if (!loaded) return;
      this.hydrated = true;
      const pending = this.pendingTimelineEvents;
      this.pendingTimelineEvents = [];
      for (const update of pending) {
        this.applyTimelineEvent(update.event, update.sourceEventId);
      }
    })();
    this.hydrationPromise = hydration;
    try {
      await hydration;
    } finally {
      if (this.hydrationPromise === hydration) this.hydrationPromise = null;
    }
  }

  private fenceRequests(): void {
    this.requestEpoch++;
    this.paginationEpoch++;
    this.isLoadingMore = false;
    this.hydrationPromise = null;
    this.urlRefreshPromise = null;
    this.pendingUrlRefreshAssetIds.clear();
  }

  private fencePagination(): boolean {
    const wasLoading = this.isLoadingMore;
    this.paginationEpoch++;
    this.isLoadingMore = false;
    return wasLoading;
  }

  private async loadPage(
    offset: number,
    replace: boolean,
    limit: number = ROOM_FILES_PAGE_SIZE,
    paginationEpoch?: number
  ): Promise<boolean> {
    const roomId = this.roomId;
    const requestEpoch = this.requestEpoch;
    let connection;
    try {
      connection = await this.attachmentAPI.listRoomAttachments({
        roomId,
        limit,
        offset,
        thumbnail: {
          width: 120,
          height: 120,
          fit: FitMode.Cover
        }
      });
    } catch (error) {
      if (
        this.requestEpoch !== requestEpoch ||
        (paginationEpoch !== undefined && this.paginationEpoch !== paginationEpoch)
      )
        return false;
      console.error('RoomFilesStore: failed to load files:', error);
      if (replace) this.isInitialLoading = false;
      return false;
    }
    if (
      this.requestEpoch !== requestEpoch ||
      (paginationEpoch !== undefined && this.paginationEpoch !== paginationEpoch)
    )
      return false;

    if (replace) {
      this.items = connection.items;
    } else {
      const seen = new SvelteSet(this.items.map(itemKey));
      this.items = [...this.items, ...connection.items.filter((item) => !seen.has(itemKey(item)))];
    }
    this.totalCount = connection.totalCount;
    this.hasMore = connection.hasMore;
    this.isInitialLoading = false;
    return true;
  }

  dispose(): void {
    this.retainCount = 0;
    this.pendingTimelineEvents = [];
    this.fenceRequests();
  }
}
