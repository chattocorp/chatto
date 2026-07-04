import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { Code, ConnectError } from '@connectrpc/connect';
import { Timestamp } from '@bufbuild/protobuf';
import { FitMode } from '$lib/api-client/renderTypes';
import {
  AssetFitMode,
  BatchGetAssetsResponse,
  RoomAttachmentListItem
} from '@chatto/api-types/api/v1/attachments_pb';
import { ListRoomAttachmentsResponse } from '@chatto/api-types/api/v1/rooms_pb';
import {
  MessageAssetUrl,
  MessageAttachment,
  MessageVideoProcessing,
  MessageVideoProcessingStatus,
  MessageVideoVariant
} from '@chatto/api-types/api/v1/message_types_pb';
import { createAttachmentAPI } from '$lib/api-client/attachments';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  listRoomAttachments: vi.fn(),
  batchGetAssets: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

function assetUrl(url: string) {
  return new MessageAssetUrl({
    url,
    expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
  });
}

describe('createAttachmentAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.listRoomAttachments.mockReset();
    mocks.batchGetAssets.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockImplementation((service) => {
      if (service.typeName === 'chatto.api.v1.RoomService') {
        return {
          listRoomAttachments: mocks.listRoomAttachments
        };
      }
      return {
        batchGetAssets: mocks.batchGetAssets
      };
    });
  });

  it('lists room attachments with bearer auth and maps attachment metadata', async () => {
    mocks.listRoomAttachments.mockResolvedValue(
      new ListRoomAttachmentsResponse({
        page: { totalCount: 2n, hasMore: true },
        attachments: [
          new RoomAttachmentListItem({
            messageEventId: 'event_2',
            threadRootEventId: 'event_1',
            createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
            attachment: new MessageAttachment({
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

    const api = createAttachmentAPI({
      serverId: 'server_1',
      baseUrl: 'https://server.test/api/connect',
      bearerToken: 'token'
    });

    const page = await api.listRoomAttachments({
      roomId: 'room_1',
      limit: 50,
      offset: 0,
      thumbnail: { width: 120, height: 120, fit: FitMode.Cover }
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://server.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.listRoomAttachments).toHaveBeenCalledWith(
      {
        roomId: 'room_1',
        page: { limit: 50, offset: 0 },
        thumbnail: { width: 120, height: 120, fit: AssetFitMode.COVER }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
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
    mocks.batchGetAssets.mockResolvedValue(
      new BatchGetAssetsResponse({
        assets: [
          new MessageAttachment({
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

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    const urls = await api.refreshAssetUrls('room_1', ['att_1'], {
      width: 960,
      height: 800,
      fit: FitMode.Contain
    });

    expect(mocks.batchGetAssets).toHaveBeenCalledWith(
      {
        roomId: 'room_1',
        assetIds: ['att_1'],
        thumbnail: { width: 960, height: 800, fit: AssetFitMode.CONTAIN }
      },
      { headers: undefined }
    );
    expect(urls.get('att_1')?.assetUrl?.url).toBe('/assets/files/att_1?fresh=1');
    expect(urls.get('att_1')?.thumbnailAssetUrl?.url).toContain('960x800');
    expect(urls.get('att_1')?.videoThumbnailAssetUrl?.url).toBe('/assets/files/thumb?fresh=1');
    expect(urls.get('att_1')?.variantAssetUrls.get('720p')?.url).toBe(
      '/assets/files/variant?fresh=1'
    );
  });

  it('keeps missing refreshed attachment URLs nullable', async () => {
    mocks.batchGetAssets.mockResolvedValue(
      new BatchGetAssetsResponse({
        assets: [
          new MessageAttachment({
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

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    const urls = await api.refreshAssetUrls('room_1', ['att_1'], {
      width: 960,
      height: 800,
      fit: FitMode.Contain
    });

    expect(urls.get('att_1')?.assetUrl).toBeNull();
    expect(urls.get('att_1')?.thumbnailAssetUrl).toBeNull();
    expect(urls.get('att_1')?.variantAssetUrls.get('720p')).toBeNull();
  });

  it('omits missing assets from refreshed URL results', async () => {
    mocks.batchGetAssets.mockResolvedValue(
      new BatchGetAssetsResponse({
        assets: [
          new MessageAttachment({
            id: 'att_1',
            assetUrl: assetUrl('/assets/files/att_1?fresh=1'),
            thumbnailAssetUrl: assetUrl('/assets/files/att_1/image/120x120/cover?fresh=1')
          })
        ]
      })
    );

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    const urls = await api.refreshAssetUrls(
      'room_1',
      ['att_1', 'missing'],
      {
        width: 120,
        height: 120,
        fit: FitMode.Cover
      }
    );

    expect(mocks.batchGetAssets).toHaveBeenCalledWith(
      {
        roomId: 'room_1',
        assetIds: ['att_1', 'missing'],
        thumbnail: { width: 120, height: 120, fit: AssetFitMode.COVER }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(urls.get('att_1')?.assetUrl?.url).toBe('/assets/files/att_1?fresh=1');
    expect(urls.has('missing')).toBe(false);
  });

  it('lists attachments with missing asset URLs as null', async () => {
    mocks.listRoomAttachments.mockResolvedValue(
      new ListRoomAttachmentsResponse({
        attachments: [
          new RoomAttachmentListItem({
            messageEventId: 'event_1',
            attachment: new MessageAttachment({
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

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    const page = await api.listRoomAttachments({
      roomId: 'room_1',
      limit: 50,
      offset: 0,
      thumbnail: { width: 120, height: 120, fit: FitMode.Cover }
    });

    expect(page.items[0]?.attachment.assetUrl).toBeNull();
    expect(page.items[0]?.attachment.videoProcessing?.variants[0]?.assetUrl).toBeNull();
  });

  it('notifies the registry when an authenticated server rejects the request', async () => {
    mocks.listRoomAttachments.mockRejectedValue(
      new ConnectError('session expired', Code.Unauthenticated)
    );

    const api = createAttachmentAPI({
      serverId: 'server_1',
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    await expect(
      api.listRoomAttachments({
        roomId: 'room_1',
        limit: 50,
        offset: 0,
        thumbnail: { width: 120, height: 120, fit: FitMode.Cover }
      })
    ).rejects.toBeInstanceOf(ConnectError);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('server_1');
  });
});
