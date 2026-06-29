import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { FitMode } from '$lib/render/types';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { AttachmentService } from '@chatto/api-types/api/v1/attachments_connect';
import {
  AttachmentFitMode,
  AttachmentThumbnailOptions
} from '@chatto/api-types/api/v1/attachments_pb';
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
    assetUrl: ExpiringAssetUrl;
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
        assetUrl: ExpiringAssetUrl;
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
};

export function createAttachmentAPI(config: AttachmentAPIConfig): AttachmentAPI {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(AttachmentService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
      serverRegistry.handleAuthenticationRequired(config.serverId);
    }
    throw err;
  }

  return {
    async listRoomAttachments({ roomId, limit, offset, thumbnail }) {
      try {
        const response = await client.listRoomAttachments(
          {
            roomId,
            page: { limit, offset },
            thumbnail: thumbnailOptions(thumbnail)
          },
          { headers: headers() }
        );
        return {
          items: response.items.map(roomFileItem),
          totalCount: Number(response.page?.totalCount ?? 0),
          hasMore: response.page?.hasMore ?? false
        };
      } catch (err) {
        return handleAuthError(err);
      }
    },
    async refreshMessageAttachmentUrls(roomId, eventId, thumbnail) {
      try {
        const response = await client.refreshMessageAttachmentUrls(
          {
            roomId,
            eventId,
            thumbnail: thumbnailOptions(thumbnail)
          },
          { headers: headers() }
        );
        return new Map(
          response.attachments.map((attachment) => [
            attachment.attachmentId,
            {
              assetUrl: assetUrl(attachment.assetUrl) ?? { url: '', expiresAt: '' },
              thumbnailAssetUrl: assetUrl(attachment.thumbnailAssetUrl),
              videoThumbnailAssetUrl: assetUrl(attachment.videoThumbnailAssetUrl),
              variantAssetUrls: new Map(
                attachment.variants
                  .map((variant) => [variant.quality, assetUrl(variant.assetUrl)] as const)
                  .filter((entry): entry is readonly [string, ExpiringAssetUrl] => entry[1] !== null)
              )
            }
          ])
        );
      } catch (err) {
        return handleAuthError(err);
      }
    }
  };
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
    assetUrl: assetUrl(value?.assetUrl) ?? { url: '', expiresAt: '' },
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
      assetUrl: assetUrl(variant.assetUrl) ?? { url: '', expiresAt: '' }
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
