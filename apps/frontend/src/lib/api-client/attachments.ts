import { authHeaders, createChattoClient, handleAuthError } from './connect.js';
import { FitMode } from './renderTypes.js';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from './attachmentUrls.js';
import {
  AttachmentFitMode,
  AttachmentThumbnailOptions
} from '@chatto/api-types/api/v1/attachments_pb';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import {
  RoomTimelineVideoProcessingStatus,
  type RoomTimelineAssetUrl,
  type RoomTimelineAttachment,
  type RoomTimelineVideoProcessing
} from '@chatto/api-types/api/v1/room_timeline_pb';

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
  refreshMessageAttachmentUrls(
    roomId: string,
    eventId: string,
    thumbnail: AttachmentRefreshOptions
  ): Promise<Map<string, RefreshedAttachmentUrls>>;
  batchRefreshMessageAttachmentUrls(
    roomId: string,
    eventIds: string[],
    thumbnail: AttachmentRefreshOptions
  ): Promise<Map<string, Map<string, RefreshedAttachmentUrls>>>;
};

export function createAttachmentAPI(config: AttachmentAPIConfig): AttachmentAPI {
  const messages = createChattoClient(MessageService, config);
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
    async refreshMessageAttachmentUrls(roomId, eventId, thumbnail) {
      try {
        const response = await messages.refreshMessageAttachmentUrls(
          {
            roomId,
            eventId,
            thumbnail: thumbnailOptions(thumbnail)
          },
          { headers: headers() }
        );
        return refreshedAttachmentUrlMap(response.attachments);
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async batchRefreshMessageAttachmentUrls(roomId, eventIds, thumbnail) {
      try {
        const response = await messages.batchRefreshMessageAttachmentUrls(
          {
            roomId,
            eventIds,
            thumbnail: thumbnailOptions(thumbnail)
          },
          { headers: headers() }
        );
        return new Map(
          response.messages.map((message) => [
            message.eventId,
            refreshedAttachmentUrlMap(message.attachments)
          ])
        );
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

function refreshedAttachmentUrlMap(
  attachments: readonly {
    attachmentId: string;
    assetUrl?: RoomTimelineAssetUrl;
    thumbnailAssetUrl?: RoomTimelineAssetUrl;
    videoThumbnailAssetUrl?: RoomTimelineAssetUrl;
    variants: readonly {
      quality: string;
      assetUrl?: RoomTimelineAssetUrl;
    }[];
  }[]
): Map<string, RefreshedAttachmentUrls> {
  return new Map(
    attachments.map((attachment) => [
      attachment.attachmentId,
      {
        assetUrl: assetUrl(attachment.assetUrl),
        thumbnailAssetUrl: assetUrl(attachment.thumbnailAssetUrl),
        videoThumbnailAssetUrl: assetUrl(attachment.videoThumbnailAssetUrl),
        variantAssetUrls: new Map(
          attachment.variants.map(
            (variant) => [variant.quality, assetUrl(variant.assetUrl)] as const
          )
        )
      }
    ])
  );
}

function thumbnailOptions(options: AttachmentRefreshOptions): AttachmentThumbnailOptions {
  return new AttachmentThumbnailOptions({
    width: options.width,
    height: options.height,
    fit: options.fit === FitMode.Contain ? AttachmentFitMode.CONTAIN : AttachmentFitMode.COVER
  });
}

function roomFileItem(item: {
  messageEventId: string;
  threadRootEventId: string;
  createdAt?: { toDate(): Date };
  attachment?: RoomTimelineAttachment;
}): RoomFileItem {
  return {
    messageEventId: item.messageEventId,
    threadRootEventId: item.threadRootEventId || null,
    createdAt: timestampToISO(item.createdAt),
    attachment: attachment(item.attachment)
  };
}

function attachment(value?: RoomTimelineAttachment): RoomFileItem['attachment'] {
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
  value?: RoomTimelineVideoProcessing
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
  status: RoomTimelineVideoProcessingStatus
): NonNullable<RoomFileItem['attachment']['videoProcessing']>['status'] | null {
  switch (status) {
    case RoomTimelineVideoProcessingStatus.PROCESSING:
      return 'PROCESSING';
    case RoomTimelineVideoProcessingStatus.COMPLETED:
      return 'COMPLETED';
    case RoomTimelineVideoProcessingStatus.FAILED:
      return 'FAILED';
    default:
      return null;
  }
}

function assetUrl(value?: RoomTimelineAssetUrl): ExpiringAssetUrl | null {
  if (!value) return null;
  return {
    url: value.url,
    expiresAt: timestampToISO(value.expiresAt)
  };
}

function timestampToISO(timestamp: { toDate(): Date } | undefined): string {
  return timestamp ? timestamp.toDate().toISOString() : '';
}
