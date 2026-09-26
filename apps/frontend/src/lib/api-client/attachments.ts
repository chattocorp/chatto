import { createChattoClient, minimumCursorHeaders, type ConnectAPIConfig } from './connect.js';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from './attachmentUrls.js';
import { ImageFitMode, ImageTransformOptions } from '@chatto/api-types/api/v1/common_pb';
import { imageFitModeOrCover } from './enumDefaults.js';
import { AssetService } from '@chatto/api-types/api/v1/attachments_connect';
import type { Asset } from '@chatto/api-types/api/v1/attachments_pb';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import {
  type Message,
  type MessageAttachment,
  MessageVideoProcessingStatus,
  type MessageAssetUrl,
  type MessageVideoProcessing
} from '@chatto/api-types/api/v1/message_types_pb';
import type { RoomTimelineEvent } from '@chatto/api-types/api/v1/room_timeline_pb';

export type AttachmentRefreshOptions = {
  width: number;
  height: number;
  fit: ImageFitMode;
};

export type RoomFileItem = {
  messageEventId: string;
  threadRootEventId: string | null;
  createdAt: string;
  attachment: {
    id: string;
    filename: string;
    contentType: string;
    description?: string | null;
    width: number;
    height: number;
    assetUrl: ExpiringAssetUrl | null;
    thumbnailAssetUrl: ExpiringAssetUrl | null;
    videoProcessing: {
      status: 'PROCESSING' | 'COMPLETED' | 'FAILED';
      durationMs: number | null;
      width: number | null;
      height: number | null;
      sourceAvailable: boolean;
      reasonCode: string | null;
      thumbnailAssetUrl: ExpiringAssetUrl | null;
      hlsMasterPlaylistUrl?: ExpiringAssetUrl | null;
      variants: Array<{
        quality: string;
        width: number;
        height: number;
        size: number;
        assetUrl: ExpiringAssetUrl | null;
      }>;
    } | null;
  };
};

export type RoomFilesPage = {
  items: RoomFileItem[];
  totalCount: number;
  hasMore: boolean;
};

export type AttachmentAPI = {
  /** Read original-file metadata without fetching the document bytes. */
  getMetadata(roomId: string, assetId: string, signal?: AbortSignal): Promise<{ size: number }>;
  listRoomAttachments(input: {
    roomId: string;
    limit: number;
    offset: number;
    thumbnail: AttachmentRefreshOptions;
    minimumCursor?: string;
  }): Promise<RoomFilesPage>;
  refreshAssetUrls(
    roomId: string,
    assetIds: string[],
    thumbnail: AttachmentRefreshOptions
  ): Promise<Map<string, RefreshedAttachmentUrls>>;
};

export function createAttachmentAPI(config: ConnectAPIConfig): AttachmentAPI {
  const assets = createChattoClient(AssetService, config);
  const rooms = createChattoClient(RoomService, config);
  return {
    async getMetadata(roomId, assetId, signal) {
      const response = await assets.getAsset({ roomId, assetId }, { signal });
      if (!response.asset) throw new Error('Asset metadata unavailable');
      return { size: Number(response.asset.size) };
    },
    async listRoomAttachments({ roomId, limit, offset, thumbnail, minimumCursor }) {
      const response = await rooms.listRoomAttachments(
        {
          roomId,
          page: { limit, offset },
          thumbnail: thumbnailOptions(thumbnail)
        },
        {
          headers: minimumCursorHeaders(minimumCursor),
          ...(minimumCursor ? { timeoutMs: 10_000 } : {})
        }
      );
      return {
        items: response.attachments.map(roomFileItem),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },
    async refreshAssetUrls(roomId, assetIds, thumbnail) {
      if (assetIds.length === 0) return new Map();
      const response = await assets.batchGetAssets({
        roomId,
        assetIds,
        thumbnail: thumbnailOptions(thumbnail)
      });
      return refreshedAttachmentUrlMap(response.assets);
    }
  };
}

function refreshedAttachmentUrlMap(
  attachments: readonly Asset[]
): Map<string, RefreshedAttachmentUrls> {
  return new Map(
    attachments.map((attachment) => [
      attachment.id,
      {
        assetUrl: assetUrl(attachment.assetUrl),
        thumbnailAssetUrl: assetUrl(attachment.thumbnailAssetUrl),
        videoThumbnailAssetUrl: assetUrl(attachment.videoProcessing?.thumbnailAssetUrl),
        hlsMasterPlaylistUrl: assetUrl(attachment.videoProcessing?.hls?.masterPlaylistUrl),
        variantAssetUrls: new Map(
          (attachment.videoProcessing?.variants ?? []).map(
            (variant) => [variant.quality, assetUrl(variant.assetUrl)] as const
          )
        )
      }
    ])
  );
}

function thumbnailOptions(options: AttachmentRefreshOptions): ImageTransformOptions {
  return new ImageTransformOptions({
    width: options.width,
    height: options.height,
    fit: imageFitModeOrCover(options.fit)
  });
}

function roomFileItem(item: {
  messageEventId: string;
  threadRootEventId: string;
  createdAt?: { toDate(): Date };
  attachment?: Asset;
  description?: string;
}): RoomFileItem {
  return {
    messageEventId: item.messageEventId,
    threadRootEventId: item.threadRootEventId || null,
    createdAt: timestampToISO(item.createdAt),
    attachment: roomFileAttachment(item.attachment, item.description)
  };
}

/** Convert one authoritative timeline message into room-file cache rows. */
export function roomFileItemsForTimelineEvent(event: RoomTimelineEvent): RoomFileItem[] {
  if (event.event.case !== 'messagePosted') return [];
  const message = event.event.value.message;
  return message ? roomFileItemsForMessage(message).map((item) => ({
    ...item, messageEventId: event.id, createdAt: timestampToISO(event.createdAt)
  })) : [];
}

/** Convert a shared authoritative message read to file-list rows. */
export function roomFileItemsForMessage(message: Message): RoomFileItem[] {
  if (!message || message.deletedAt) return [];
  return message.attachments.map((attachment) => ({
    messageEventId: message.id,
    threadRootEventId: message.threadRootEventId || null,
    createdAt: timestampToISO(message.createdAt),
    attachment: roomFileAttachment(attachment, attachment.description)
  }));
}

function roomFileAttachment(
  value?: Asset | MessageAttachment,
  description?: string
): RoomFileItem['attachment'] {
  return {
    id: value?.id ?? '',
    filename: value?.filename ?? '',
    contentType: value?.contentType ?? '',
    description: description ?? null,
    width: value?.width ?? 0,
    height: value?.height ?? 0,
    assetUrl: assetUrl(value?.assetUrl),
    thumbnailAssetUrl: assetUrl(value?.thumbnailAssetUrl),
    videoProcessing: videoProcessing(value?.videoProcessing)
  };
}

function videoProcessing(
  value?: MessageVideoProcessing
): NonNullable<RoomFileItem['attachment']['videoProcessing']> | null {
  if (!value) return null;
  const status = videoProcessingStatus(value.status);
  if (!status) return null;
  return {
    status,
    durationMs: Number(value.durationMs) || null,
    width: value.width || null,
    height: value.height || null,
    sourceAvailable: value.sourceAvailable,
    reasonCode: value.reasonCode || null,
    thumbnailAssetUrl: assetUrl(value.thumbnailAssetUrl),
    hlsMasterPlaylistUrl: assetUrl(value.hls?.masterPlaylistUrl),
    variants: value.variants.map((variant) => ({
      quality: variant.quality,
      width: variant.width,
      height: variant.height,
      size: Number(variant.size),
      assetUrl: assetUrl(variant.assetUrl)
    }))
  };
}

function videoProcessingStatus(
  status: MessageVideoProcessingStatus
): NonNullable<RoomFileItem['attachment']['videoProcessing']>['status'] | null {
  switch (status) {
    case MessageVideoProcessingStatus.PROCESSING:
      return 'PROCESSING';
    case MessageVideoProcessingStatus.COMPLETED:
      return 'COMPLETED';
    case MessageVideoProcessingStatus.FAILED:
      return 'FAILED';
    default:
      return null;
  }
}

function assetUrl(value?: MessageAssetUrl): ExpiringAssetUrl | null {
  if (!value) return null;
  return {
    url: value.url,
    expiresAt: timestampToISO(value.expiresAt)
  };
}

function timestampToISO(timestamp: { toDate(): Date } | undefined): string {
  return timestamp ? timestamp.toDate().toISOString() : '';
}
