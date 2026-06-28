import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { FitMode } from '$lib/render/types';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import {
  assetUrlNeedsRefresh,
  earliestAssetUrlRefreshAt,
  mergeRefreshedAttachmentUrls,
  refreshAttachmentUrlsForMessage
} from '$lib/attachments/attachmentUrls';
import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { createAttachmentAPI, type AttachmentAPI, type RoomFileItem } from '$lib/api/attachments';

export const ROOM_FILES_PAGE_SIZE = 50;

export type { RoomFileItem };

function itemKey(item: RoomFileItem): string {
  return `${item.messageEventId}:${item.attachment.id}`;
}

const roomFilesInvalidatingEventKinds = new Set<RoomEventKind>([
  RoomEventKind.MessagePosted,
  RoomEventKind.MessageEdited,
  RoomEventKind.MessageRetracted,
  RoomEventKind.AssetProcessingStarted,
  RoomEventKind.AssetProcessingSucceeded,
  RoomEventKind.AssetProcessingFailed,
  RoomEventKind.AssetDeleted
]);

function attachmentAssetUrls(item: RoomFileItem, refreshed: RefreshedAttachmentUrls | undefined) {
  return [
    refreshed?.assetUrl ?? item.attachment.assetUrl,
    refreshed?.thumbnailAssetUrl ?? item.attachment.thumbnailAssetUrl,
    refreshed?.videoThumbnailAssetUrl ?? item.attachment.videoProcessing?.thumbnailAssetUrl
  ];
}

function eventRoomId(eventData: EventEnvelope['event']): string | null {
  if (!eventData) return null;
  if ('roomId' in eventData) return eventData.roomId ?? null;
  if ('processingRoomId' in eventData) return eventData.processingRoomId ?? null;
  if ('deletedRoomId' in eventData) return eventData.deletedRoomId ?? null;
  return null;
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
  isUnsupported = $state(false);
  refreshedAttachmentUrls = new SvelteMap<string, RefreshedAttachmentUrls>();

  private readonly attachmentAPI: AttachmentAPI;
  private roomId = '';
  #loadId = 0;

  constructor(serverConnection: ServerConnection) {
    this.attachmentAPI = createAttachmentAPI({
      serverId: serverConnection.serverId,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    });
  }

  setRoom(roomId: string): void {
    if (this.roomId === roomId) return;
    this.roomId = roomId;
    this.items = [];
    this.totalCount = 0;
    this.hasMore = false;
    this.isUnsupported = false;
    this.refreshedAttachmentUrls = new SvelteMap();
    void this.loadInitial();
  }

  async loadInitial(): Promise<void> {
    if (!this.roomId || this.isUnsupported) return;
    this.isInitialLoading = true;
    await this.loadPage(0, true);
  }

  async loadMore(): Promise<void> {
    if (this.isLoadingMore || !this.hasMore || !this.roomId || this.isUnsupported) return;
    this.isLoadingMore = true;
    try {
      await this.loadPage(this.items.length, false);
    } finally {
      this.isLoadingMore = false;
    }
  }

  async refresh(): Promise<void> {
    if (!this.roomId || this.isUnsupported) return;
    await this.loadPage(0, true, Math.max(ROOM_FILES_PAGE_SIZE, this.items.length));
  }

  ingestServerEvent(serverEvent: EventEnvelope): void {
    const eventData = serverEvent.event;
    if (!eventData) return;
    if (eventRoomId(eventData) !== this.roomId) return;

    const kind = roomEventKind(eventData);
    if (kind && roomFilesInvalidatingEventKinds.has(kind)) {
      void this.refresh();
    }
  }

  assetUrlFor(item: RoomFileItem): ExpiringAssetUrl {
    return (
      this.refreshedAttachmentUrls.get(item.attachment.id)?.assetUrl ?? item.attachment.assetUrl
    );
  }

  thumbnailAssetUrlFor(item: RoomFileItem): ExpiringAssetUrl | null {
    const refreshed = this.refreshedAttachmentUrls.get(item.attachment.id);
    const contentType = item.attachment.contentType;
    if (isVideoAttachment(contentType)) {
      return (
        refreshed?.videoThumbnailAssetUrl ??
        item.attachment.videoProcessing?.thumbnailAssetUrl ??
        null
      );
    }
    if (!isImageAttachment(contentType)) return null;

    return (
      refreshed?.thumbnailAssetUrl ??
      refreshed?.videoThumbnailAssetUrl ??
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
    if (!this.roomId || this.isUnsupported || items.length === 0) return;
    const eventIds = Array.from(new SvelteSet(items.map((item) => item.messageEventId)));
    const freshMaps = await Promise.all(
      eventIds.map((eventId) =>
        refreshAttachmentUrlsForMessage(this.attachmentAPI, this.roomId, eventId, {
          width: 120,
          height: 120,
          fit: FitMode.Cover
        })
      )
    );
    const fresh = new SvelteMap<string, RefreshedAttachmentUrls>();
    for (const freshMap of freshMaps) {
      for (const [attachmentId, urls] of freshMap) {
        fresh.set(attachmentId, urls);
      }
    }
    this.refreshedAttachmentUrls = new SvelteMap(
      mergeRefreshedAttachmentUrls(this.refreshedAttachmentUrls, fresh)
    );
  }

  private async loadPage(
    offset: number,
    replace: boolean,
    limit: number = ROOM_FILES_PAGE_SIZE
  ): Promise<void> {
    const roomId = this.roomId;
    const thisLoad = ++this.#loadId;
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
      if (this.#loadId !== thisLoad || this.roomId !== roomId) return;
      console.error('RoomFilesStore: failed to load files:', error);
      if (replace) this.isInitialLoading = false;
      return;
    }
    if (this.#loadId !== thisLoad || this.roomId !== roomId) return;

    if (replace) {
      this.items = connection.items;
    } else {
      const seen = new SvelteSet(this.items.map(itemKey));
      this.items = [...this.items, ...connection.items.filter((item) => !seen.has(itemKey(item)))];
    }
    this.totalCount = connection.totalCount;
    this.hasMore = connection.hasMore;
    this.isInitialLoading = false;
  }
}
