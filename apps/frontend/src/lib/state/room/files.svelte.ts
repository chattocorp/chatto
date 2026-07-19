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
import {
  createAttachmentAPI,
  type AttachmentAPI,
  type RoomFileItem
} from '$lib/api-client/attachments';

export const ROOM_FILES_PAGE_SIZE = 50;

export type { RoomFileItem };

function itemKey(item: RoomFileItem): string {
  return `${item.messageEventId}:${item.attachment.id}`;
}

function attachmentAssetUrls(item: RoomFileItem, refreshed: RefreshedAttachmentUrls | undefined) {
  if (refreshed) {
    return [refreshed.assetUrl, refreshed.thumbnailAssetUrl, refreshed.videoThumbnailAssetUrl];
  }

  return [
    item.attachment.assetUrl,
    item.attachment.thumbnailAssetUrl,
    item.attachment.videoProcessing?.thumbnailAssetUrl
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
  private roomId = '';
  private active = false;
  private invalidationGeneration = 0;
  private loadedGeneration = -1;
  private requestEpoch = 0;
  private replacementPromise: Promise<void> | null = null;
  private urlRefreshPromise: Promise<void> | null = null;
  private pendingUrlRefreshAssetIds = new SvelteSet<string>();
  #loadId = 0;

  constructor(serverConnection: ServerConnection) {
    this.attachmentAPI = createAttachmentAPI({
      serverId: serverConnection.serverId,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    });
  }

  /** Select a room without loading its Files panel. */
  selectRoom(roomId: string): void {
    if (this.roomId === roomId) return;
    this.fenceRequests();
    this.roomId = roomId;
    this.items = [];
    this.totalCount = 0;
    this.hasMore = false;
    this.isInitialLoading = true;
    this.isLoadingMore = false;
    this.refreshedAttachmentUrls = new SvelteMap();
    this.invalidationGeneration++;
    this.loadedGeneration = -1;
  }

  /** Activate the files read model for the visible room Files panel. */
  activate(roomId?: string): void {
    if (roomId) this.selectRoom(roomId);

    this.active = true;
    void this.ensureFresh();
  }

  /** Stop network refreshes while the Files panel is not visible. */
  deactivate(): void {
    if (!this.active) return;
    this.active = false;
    this.fenceRequests();
  }

  /** Refresh visible files, or remember that dormant data must be refreshed later. */
  invalidate(roomId: string): void {
    if (roomId !== this.roomId) return;
    this.invalidationGeneration++;
    if (this.active) void this.ensureFresh();
  }

  async loadMore(): Promise<void> {
    if (
      !this.active ||
      this.replacementPromise ||
      this.isLoadingMore ||
      !this.hasMore ||
      !this.roomId
    )
      return;
    const roomId = this.roomId;
    const requestEpoch = this.requestEpoch;
    this.isLoadingMore = true;
    try {
      await this.loadPage(this.items.length, false);
    } finally {
      if (this.roomId === roomId && this.requestEpoch === requestEpoch) {
        this.isLoadingMore = false;
      }
    }
  }

  async refresh(): Promise<void> {
    if (!this.active || !this.roomId) return;
    this.invalidationGeneration++;
    await this.ensureFresh();
  }

  hasFilesForMessage(messageEventId: string): boolean {
    return this.items.some((item) => item.messageEventId === messageEventId);
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
    if (!this.active || !this.roomId || items.length === 0) return;
    for (const item of items) this.pendingUrlRefreshAssetIds.add(item.attachment.id);
    if (this.urlRefreshPromise) return this.urlRefreshPromise;

    const roomId = this.roomId;
    const requestEpoch = this.requestEpoch;
    const refresh = (async () => {
      while (
        this.active &&
        this.roomId === roomId &&
        this.requestEpoch === requestEpoch &&
        this.pendingUrlRefreshAssetIds.size > 0
      ) {
        const assetIds = Array.from(this.pendingUrlRefreshAssetIds);
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
        if (!this.active || this.roomId !== roomId || this.requestEpoch !== requestEpoch) return;
        for (const assetId of assetIds) this.pendingUrlRefreshAssetIds.delete(assetId);

        const fresh = new SvelteMap<string, RefreshedAttachmentUrls>();
        for (const [attachmentId, urls] of freshMap) {
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
    if (!this.active || !this.roomId || this.loadedGeneration === this.invalidationGeneration)
      return;
    if (this.replacementPromise) return this.replacementPromise;

    const replacement = (async () => {
      while (this.active && this.roomId && this.loadedGeneration !== this.invalidationGeneration) {
        const generation = this.invalidationGeneration;
        if (this.items.length === 0) this.isInitialLoading = true;
        const loaded = await this.loadPage(
          0,
          true,
          Math.max(ROOM_FILES_PAGE_SIZE, this.items.length),
          generation
        );
        if (!loaded) break;
      }
    })();
    this.replacementPromise = replacement;
    try {
      await replacement;
    } finally {
      if (this.replacementPromise === replacement) this.replacementPromise = null;
    }
  }

  private fenceRequests(): void {
    this.requestEpoch++;
    this.#loadId++;
    this.isLoadingMore = false;
    this.replacementPromise = null;
    this.urlRefreshPromise = null;
    this.pendingUrlRefreshAssetIds.clear();
  }

  private async loadPage(
    offset: number,
    replace: boolean,
    limit: number = ROOM_FILES_PAGE_SIZE,
    generation: number = this.invalidationGeneration
  ): Promise<boolean> {
    const roomId = this.roomId;
    const thisLoad = ++this.#loadId;
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
        this.#loadId !== thisLoad ||
        this.roomId !== roomId ||
        this.requestEpoch !== requestEpoch
      )
        return false;
      console.error('RoomFilesStore: failed to load files:', error);
      if (replace) this.isInitialLoading = false;
      return false;
    }
    if (
      !this.active ||
      this.#loadId !== thisLoad ||
      this.roomId !== roomId ||
      this.requestEpoch !== requestEpoch
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
    if (replace) this.loadedGeneration = generation;
    this.isInitialLoading = false;
    return true;
  }
}
