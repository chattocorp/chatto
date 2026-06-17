import type { Client } from '@urql/svelte';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { graphql } from '$lib/gql';
import { isUnsupportedGraphQLFieldError } from '$lib/gql/compatibility';
import { FitMode, RoomFilesDocument, type RoomFilesQuery } from '$lib/gql/graphql';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import {
  assetUrlNeedsRefresh,
  earliestAssetUrlRefreshAt,
  mergeRefreshedAttachmentUrls,
  refreshAttachmentUrlsForMessage
} from '$lib/attachments/attachmentUrls';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';

export const ROOM_FILES_PAGE_SIZE = 50;

const RoomFilesQueryDocument = graphql(`
  query RoomFiles($roomId: ID!, $limit: Int!, $offset: Int!) {
    room(roomId: $roomId) {
      attachments(limit: $limit, offset: $offset) {
        items {
          messageEventId
          threadRootEventId
          createdAt
          attachment {
            id
            filename
            contentType
            width
            height
            assetUrl {
              url
              expiresAt
            }
            thumbnailAssetUrl(width: 120, height: 120, fit: COVER) {
              url
              expiresAt
            }
            videoProcessing {
              status
              thumbnailAssetUrl {
                url
                expiresAt
              }
            }
          }
        }
        totalCount
        hasMore
      }
    }
  }
`) as typeof RoomFilesDocument;

export type RoomFileItem = NonNullable<
  NonNullable<RoomFilesQuery['room']>['attachments']['items'][number]
>;

function itemKey(item: RoomFileItem): string {
  return `${item.messageEventId}:${item.attachment.id}`;
}

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

  private readonly client: Client;
  private roomId = '';
  #loadId = 0;

  constructor(gqlClient: GraphQLClient) {
    this.client = gqlClient.client;
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

    if (
      eventData.__typename === 'MessagePostedEvent' ||
      eventData.__typename === 'MessageEditedEvent' ||
      eventData.__typename === 'MessageRetractedEvent' ||
      eventData.__typename === 'AssetProcessingStartedEvent' ||
      eventData.__typename === 'AssetProcessingSucceededEvent' ||
      eventData.__typename === 'AssetProcessingFailedEvent' ||
      eventData.__typename === 'AssetDeletedEvent'
    ) {
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
        refreshAttachmentUrlsForMessage(this.client, this.roomId, eventId, {
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
    const result = await this.client
      .query(RoomFilesQueryDocument, {
        roomId,
        limit,
        offset
      })
      .toPromise();

    if (this.#loadId !== thisLoad || this.roomId !== roomId) return;
    if (result.error) {
      if (isUnsupportedGraphQLFieldError(result.error, 'attachments')) {
        this.isUnsupported = true;
        this.items = [];
        this.totalCount = 0;
        this.hasMore = false;
        if (replace) this.isInitialLoading = false;
        return;
      }
      console.error('RoomFilesStore: failed to load files:', result.error);
      if (replace) this.isInitialLoading = false;
      return;
    }

    const connection = result.data?.room?.attachments;
    if (!connection) {
      if (replace) {
        this.items = [];
        this.totalCount = 0;
        this.hasMore = false;
        this.isInitialLoading = false;
      }
      return;
    }

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
