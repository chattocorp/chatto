import { authHeaders, createChattoClient, handleAuthError } from './connect.js';
import { FitMode } from './renderTypes.js';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from './attachmentUrls.js';
import { ImageFitMode, ImageTransformOptions } from '@chatto/api-types/api/v1/common_pb';
import { AssetService } from '@chatto/api-types/api/v1/attachments_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import {
  MessageVideoProcessingStatus,
  type MessageAssetUrl,
  type MessageAttachment,
  type MessageVideoProcessing
} from '@chatto/api-types/api/v1/message_types_pb';

export type AttachmentAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type AttachmentRefreshOptions = {
  width: number;
  height: number;
  fit: FitMode;
};

export type RoomFileItem = {
  messageEventId: string;
  threadRootEventId: string | null;
  createdAt: string;
  attachment: {
    id: string;
    filename: string;
    contentType: string;
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
  listRoomAttachments(input: {
    roomId: string;
    limit: number;
    offset: number;
    thumbnail: AttachmentRefreshOptions;
  }): Promise<RoomFilesPage>;
  refreshAssetUrls(
    roomId: string,
    assetIds: string[],
    thumbnail: AttachmentRefreshOptions
  ): Promise<Map<string, RefreshedAttachmentUrls>>;
};

export function createAttachmentAPI(config: AttachmentAPIConfig): AttachmentAPI {
  const assets = createChattoClient(AssetService, config);
  const rooms = createChattoClient(RoomService, config);
  const headers = () => authHeaders(config);
  return {
    async listRoomAttachments({ roomId, limit, offset, thumbnail }) {
      try {
        const response = await rooms.listRoomAttachments(
          {
            roomId,
            page: { limit, offset },
            thumbnail: thumbnailOptions(thumbnail)
          },
          { headers: headers() }
        );
        return {
          items: response.attachments.map(roomFileItem),
          totalCount: Number(response.page?.totalCount ?? 0),
          hasMore: response.page?.hasMore ?? false
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async refreshAssetUrls(roomId, assetIds, thumbnail) {
      if (assetIds.length === 0) return new Map();
      try {
        const response = await assets.batchGetAssets(
          {
            roomId,
            assetIds,
            thumbnail: thumbnailOptions(thumbnail)
          },
          { headers: headers() }
        );
        return refreshedAttachmentUrlMap(response.assets);
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

function refreshedAttachmentUrlMap(
  attachments: readonly MessageAttachment[]
): Map<string, RefreshedAttachmentUrls> {
  return new Map(
    attachments.map((attachment) => [
      attachment.id,
      {
        assetUrl: assetUrl(attachment.assetUrl),
        thumbnailAssetUrl: assetUrl(attachment.thumbnailAssetUrl),
        videoThumbnailAssetUrl: assetUrl(attachment.videoProcessing?.thumbnailAssetUrl),
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
    fit: options.fit === FitMode.Contain ? ImageFitMode.CONTAIN : ImageFitMode.COVER
  });
}

function roomFileItem(item: {
  messageEventId: string;
  threadRootEventId: string;
  createdAt?: { toDate(): Date };
  attachment?: MessageAttachment;
}): RoomFileItem {
  return {
    messageEventId: item.messageEventId,
    threadRootEventId: item.threadRootEventId || null,
    createdAt: timestampToISO(item.createdAt),
    attachment: attachment(item.attachment)
  };
}

function attachment(value?: MessageAttachment): RoomFileItem['attachment'] {
  return {
    id: value?.id ?? '',
    filename: value?.filename ?? '',
    contentType: value?.contentType ?? '',
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
