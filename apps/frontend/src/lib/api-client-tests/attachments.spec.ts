import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { Timestamp } from '@bufbuild/protobuf';

import {
  Asset,
  GetAssetResponse,
  BatchGetAssetsResponse,
  RoomAttachmentListItem
} from '@chatto/api-types/api/v1/attachments_pb';
import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
import { ListRoomAttachmentsResponse } from '@chatto/api-types/api/v1/rooms_pb';
import {
  MessageAssetUrl,
  MessageVideoProcessing,
  MessageVideoProcessingStatus,
  MessageVideoVariant
} from '@chatto/api-types/api/v1/message_types_pb';
import { createAttachmentAPI } from '$lib/api-client/attachments';
import { AssetService } from '@chatto/api-types/api/v1/attachments_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const assets = mockService(AssetService);
const rooms = mockService(RoomService);

function attachmentAPI() {
  return createAttachmentAPI(
    fakeServer((router) => router.service(AssetService, assets).service(RoomService, rooms))
  );
}

function assetUrl(url: string) {
  return new MessageAssetUrl({
    url,
    expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
  });
}

describe('createAttachmentAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('reads file size metadata and forwards cancellation', async () => {
    assets.getAsset.mockReturnValue(
      new GetAssetResponse({ asset: new Asset({ id: 'html', size: 1536n }) })
    );
    const api = attachmentAPI();
    await expect(api.getMetadata('room', 'html')).resolves.toEqual({ size: 1536 });
    expect(receivedRequest(assets.getAsset)).toMatchObject({ roomId: 'room', assetId: 'html' });

    await expect(api.getMetadata('room', 'html', AbortSignal.abort())).rejects.toMatchObject({
      code: Code.Canceled
    });
  });

  it('does not report a missing asset as a zero-byte file', async () => {
    assets.getAsset.mockReturnValue(new GetAssetResponse());
    const api = attachmentAPI();
    await expect(api.getMetadata('room', 'missing')).rejects.toThrow('Asset metadata unavailable');
  });

  it('lists room attachments and maps attachment metadata', async () => {
    rooms.listRoomAttachments.mockReturnValue(
      new ListRoomAttachmentsResponse({
        page: { totalCount: 2n, hasMore: true },
        attachments: [
          new RoomAttachmentListItem({
            messageEventId: 'event_2',
            threadRootEventId: 'event_1',
            description: 'A short video clip',
            createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
            attachment: new Asset({
              id: 'att_video',
              filename: 'clip.mp4',
              contentType: 'video/mp4',
              width: 1280,
              height: 720,
              assetUrl: assetUrl('/assets/files/att_video'),
              thumbnailAssetUrl: assetUrl('/assets/files/att_video/image/120x120/cover'),
              videoProcessing: new MessageVideoProcessing({
                status: MessageVideoProcessingStatus.COMPLETED,
                durationMs: 1234n,
                width: 1280,
                height: 720,
                sourceAvailable: true,
                thumbnailAssetUrl: assetUrl('/assets/files/att_thumb'),
                variants: [
                  new MessageVideoVariant({
                    quality: '720p',
                    width: 1280,
                    height: 720,
                    size: 4567n,
                    assetUrl: assetUrl('/assets/files/att_variant')
                  })
                ]
              })
            })
          })
        ]
      })
    );

    const api = attachmentAPI();

    const page = await api.listRoomAttachments({
      roomId: 'room_1',
      limit: 50,
      offset: 0,
      thumbnail: { width: 120, height: 120, fit: ImageFitMode.COVER }
    });

    expect(receivedRequest(rooms.listRoomAttachments)).toMatchObject({
      roomId: 'room_1',
      page: { limit: 50, offset: 0 },
      thumbnail: { width: 120, height: 120, fit: ImageFitMode.COVER }
    });
    expect(page).toMatchObject({
      totalCount: 2,
      hasMore: true,
      items: [
        {
          messageEventId: 'event_2',
          threadRootEventId: 'event_1',
          createdAt: '2026-06-01T12:00:00.000Z',
          attachment: {
            id: 'att_video',
            filename: 'clip.mp4',
            contentType: 'video/mp4',
            description: 'A short video clip',
            assetUrl: { url: '/assets/files/att_video' },
            thumbnailAssetUrl: { url: '/assets/files/att_video/image/120x120/cover' },
            videoProcessing: {
              status: 'COMPLETED',
              durationMs: 1234,
              variants: [{ quality: '720p', assetUrl: { url: '/assets/files/att_variant' } }]
            }
          }
        }
      ]
    });
  });

  it('refreshes asset URLs and maps video variants', async () => {
    assets.batchGetAssets.mockReturnValue(
      new BatchGetAssetsResponse({
        assets: [
          new Asset({
            id: 'att_1',
            assetUrl: assetUrl('/assets/files/att_1?fresh=1'),
            thumbnailAssetUrl: assetUrl('/assets/files/att_1/image/960x800/contain?fresh=1'),
            videoProcessing: new MessageVideoProcessing({
              status: MessageVideoProcessingStatus.COMPLETED,
              thumbnailAssetUrl: assetUrl('/assets/files/thumb?fresh=1'),
              variants: [
                new MessageVideoVariant({
                  quality: '720p',
                  width: 1280,
                  height: 720,
                  size: 4567n,
                  assetUrl: assetUrl('/assets/files/variant?fresh=1')
                })
              ]
            })
          })
        ]
      })
    );

    const api = attachmentAPI();

    const urls = await api.refreshAssetUrls('room_1', ['att_1'], {
      width: 960,
      height: 800,
      fit: ImageFitMode.CONTAIN
    });

    expect(receivedRequest(assets.batchGetAssets)).toMatchObject({
      roomId: 'room_1',
      assetIds: ['att_1'],
      thumbnail: { width: 960, height: 800, fit: ImageFitMode.CONTAIN }
    });
    expect(urls.get('att_1')?.assetUrl?.url).toBe('/assets/files/att_1?fresh=1');
    expect(urls.get('att_1')?.thumbnailAssetUrl?.url).toContain('960x800');
    expect(urls.get('att_1')?.videoThumbnailAssetUrl?.url).toBe('/assets/files/thumb?fresh=1');
    expect(urls.get('att_1')?.variantAssetUrls.get('720p')?.url).toBe(
      '/assets/files/variant?fresh=1'
    );
  });

  it('keeps missing refreshed attachment URLs nullable', async () => {
    assets.batchGetAssets.mockReturnValue(
      new BatchGetAssetsResponse({
        assets: [
          new Asset({
            id: 'att_1',
            videoProcessing: new MessageVideoProcessing({
              status: MessageVideoProcessingStatus.COMPLETED,
              variants: [
                new MessageVideoVariant({
                  quality: '720p',
                  width: 1280,
                  height: 720,
                  size: 4567n
                })
              ]
            })
          })
        ]
      })
    );

    const api = attachmentAPI();

    const urls = await api.refreshAssetUrls('room_1', ['att_1'], {
      width: 960,
      height: 800,
      fit: ImageFitMode.CONTAIN
    });

    expect(urls.get('att_1')?.assetUrl).toBeNull();
    expect(urls.get('att_1')?.thumbnailAssetUrl).toBeNull();
    expect(urls.get('att_1')?.variantAssetUrls.get('720p')).toBeNull();
  });

  it('omits missing assets from refreshed URL results', async () => {
    assets.batchGetAssets.mockReturnValue(
      new BatchGetAssetsResponse({
        assets: [
          new Asset({
            id: 'att_1',
            assetUrl: assetUrl('/assets/files/att_1?fresh=1'),
            thumbnailAssetUrl: assetUrl('/assets/files/att_1/image/120x120/cover?fresh=1')
          })
        ]
      })
    );

    const api = attachmentAPI();

    const urls = await api.refreshAssetUrls('room_1', ['att_1', 'missing'], {
      width: 120,
      height: 120,
      fit: ImageFitMode.COVER
    });

    expect(receivedRequest(assets.batchGetAssets)).toMatchObject({
      roomId: 'room_1',
      assetIds: ['att_1', 'missing'],
      thumbnail: { width: 120, height: 120, fit: ImageFitMode.COVER }
    });
    expect(urls.get('att_1')?.assetUrl?.url).toBe('/assets/files/att_1?fresh=1');
    expect(urls.has('missing')).toBe(false);
  });

  it('lists attachments with missing asset URLs as null', async () => {
    rooms.listRoomAttachments.mockReturnValue(
      new ListRoomAttachmentsResponse({
        attachments: [
          new RoomAttachmentListItem({
            messageEventId: 'event_1',
            attachment: new Asset({
              id: 'att_1',
              filename: 'clip.mp4',
              contentType: 'video/mp4',
              videoProcessing: new MessageVideoProcessing({
                status: MessageVideoProcessingStatus.COMPLETED,
                variants: [
                  new MessageVideoVariant({
                    quality: '720p',
                    width: 1280,
                    height: 720,
                    size: 4567n
                  })
                ]
              })
            })
          })
        ]
      })
    );

    const api = attachmentAPI();

    const page = await api.listRoomAttachments({
      roomId: 'room_1',
      limit: 50,
      offset: 0,
      thumbnail: { width: 120, height: 120, fit: ImageFitMode.COVER }
    });

    expect(page.items[0]?.attachment.assetUrl).toBeNull();
    expect(page.items[0]?.attachment.videoProcessing?.variants[0]?.assetUrl).toBeNull();
  });

  it('propagates Connect errors', async () => {
    rooms.listRoomAttachments.mockImplementation(() => {
      throw new ConnectError('session expired', Code.Unauthenticated);
    });

    const api = attachmentAPI();

    await expect(
      api.listRoomAttachments({
        roomId: 'room_1',
        limit: 50,
        offset: 0,
        thumbnail: { width: 120, height: 120, fit: ImageFitMode.COVER }
      })
    ).rejects.toMatchObject({ code: Code.Unauthenticated, rawMessage: 'session expired' });
  });
});
