import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { Code, ConnectError } from '@connectrpc/connect';
import { Timestamp } from '@bufbuild/protobuf';
import { FitMode } from '$lib/api-client/renderTypes';
import {
  AttachmentFitMode,
  RefreshedAttachmentUrls,
  RoomAttachmentListItem
} from '@chatto/api-types/api/v1/attachments_pb';
import {
  BatchRefreshMessageAttachmentUrlsResponse,
  RefreshedMessageAttachmentUrls,
  RefreshMessageAttachmentUrlsResponse
} from '@chatto/api-types/api/v1/messages_pb';
import { ListRoomAttachmentsResponse } from '@chatto/api-types/api/v1/rooms_pb';
import {
  RoomTimelineAssetUrl,
  RoomTimelineAttachment,
  RoomTimelineVideoProcessing,
  RoomTimelineVideoProcessingStatus,
  RoomTimelineVideoVariant
} from '@chatto/api-types/api/v1/room_timeline_pb';
import { createAttachmentAPI } from '$lib/api-client/attachments';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  listRoomAttachments: vi.fn(),
  refreshMessageAttachmentUrls: vi.fn(),
  batchRefreshMessageAttachmentUrls: vi.fn()
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
  return new RoomTimelineAssetUrl({
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
    mocks.refreshMessageAttachmentUrls.mockReset();
    mocks.batchRefreshMessageAttachmentUrls.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockImplementation((service) => {
      if (service.typeName === 'chatto.api.v1.RoomService') {
        return {
          listRoomAttachments: mocks.listRoomAttachments
        };
      }
      return {
        refreshMessageAttachmentUrls: mocks.refreshMessageAttachmentUrls,
        batchRefreshMessageAttachmentUrls: mocks.batchRefreshMessageAttachmentUrls
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
            attachment: new RoomTimelineAttachment({
              id: 'att_video',
              filename: 'clip.mp4',
              contentType: 'video/mp4',
              width: 1280,
              height: 720,
              assetUrl: assetUrl('/assets/files/att_video'),
              thumbnailAssetUrl: assetUrl('/assets/files/att_video/image/120x120/cover'),
              videoProcessing: new RoomTimelineVideoProcessing({
                status: RoomTimelineVideoProcessingStatus.COMPLETED,
                durationMs: 1234n,
                width: 1280,
                height: 720,
                sourceAvailable: true,
                thumbnailAssetUrl: assetUrl('/assets/files/att_thumb'),
                variants: [
                  new RoomTimelineVideoVariant({
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
        thumbnail: { width: 120, height: 120, fit: AttachmentFitMode.COVER }
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

  it('refreshes message attachment URLs and maps video variants', async () => {
    mocks.refreshMessageAttachmentUrls.mockResolvedValue(
      new RefreshMessageAttachmentUrlsResponse({
        attachments: [
          new RefreshedAttachmentUrls({
            attachmentId: 'att_1',
            assetUrl: assetUrl('/assets/files/att_1?fresh=1'),
            thumbnailAssetUrl: assetUrl('/assets/files/att_1/image/960x800/contain?fresh=1'),
            videoThumbnailAssetUrl: assetUrl('/assets/files/thumb?fresh=1'),
            variants: [
              new RoomTimelineVideoVariant({
                quality: '720p',
                width: 1280,
                height: 720,
                size: 4567n,
                assetUrl: assetUrl('/assets/files/variant?fresh=1')
              })
            ]
          })
        ]
      })
    );

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    const urls = await api.refreshMessageAttachmentUrls('room_1', 'event_1', {
      width: 960,
      height: 800,
      fit: FitMode.Contain
    });

    expect(mocks.refreshMessageAttachmentUrls).toHaveBeenCalledWith(
      {
        roomId: 'room_1',
        eventId: 'event_1',
        thumbnail: { width: 960, height: 800, fit: AttachmentFitMode.CONTAIN }
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
    mocks.refreshMessageAttachmentUrls.mockResolvedValue(
      new RefreshMessageAttachmentUrlsResponse({
        attachments: [
          new RefreshedAttachmentUrls({
            attachmentId: 'att_1',
            variants: [
              new RoomTimelineVideoVariant({
                quality: '720p',
                width: 1280,
                height: 720,
                size: 4567n
              })
            ]
          })
        ]
      })
    );

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    const urls = await api.refreshMessageAttachmentUrls('room_1', 'event_1', {
      width: 960,
      height: 800,
      fit: FitMode.Contain
    });

    expect(urls.get('att_1')?.assetUrl).toBeNull();
    expect(urls.get('att_1')?.thumbnailAssetUrl).toBeNull();
    expect(urls.get('att_1')?.variantAssetUrls.get('720p')).toBeNull();
  });

  it('batch refreshes message attachment URLs', async () => {
    mocks.batchRefreshMessageAttachmentUrls.mockResolvedValue(
      new BatchRefreshMessageAttachmentUrlsResponse({
        messages: [
          new RefreshedMessageAttachmentUrls({
            eventId: 'event_1',
            attachments: [
              new RefreshedAttachmentUrls({
                attachmentId: 'att_1',
                assetUrl: assetUrl('/assets/files/att_1?fresh=1'),
                thumbnailAssetUrl: assetUrl('/assets/files/att_1/image/120x120/cover?fresh=1')
              })
            ]
          }),
          new RefreshedMessageAttachmentUrls({
            eventId: 'event_2',
            attachments: []
          })
        ]
      })
    );

    const api = createAttachmentAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    const messages = await api.batchRefreshMessageAttachmentUrls(
      'room_1',
      ['event_1', 'missing', 'event_2'],
      {
        width: 120,
        height: 120,
        fit: FitMode.Cover
      }
    );

    expect(mocks.batchRefreshMessageAttachmentUrls).toHaveBeenCalledWith(
      {
        roomId: 'room_1',
        eventIds: ['event_1', 'missing', 'event_2'],
        thumbnail: { width: 120, height: 120, fit: AttachmentFitMode.COVER }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(messages.get('event_1')?.get('att_1')?.assetUrl?.url).toBe(
      '/assets/files/att_1?fresh=1'
    );
    expect(messages.get('event_2')?.size).toBe(0);
    expect(messages.has('missing')).toBe(false);
  });

  it('lists attachments with missing asset URLs as null', async () => {
    mocks.listRoomAttachments.mockResolvedValue(
      new ListRoomAttachmentsResponse({
        attachments: [
          new RoomAttachmentListItem({
            messageEventId: 'event_1',
            attachment: new RoomTimelineAttachment({
              id: 'att_1',
              filename: 'clip.mp4',
              contentType: 'video/mp4',
              videoProcessing: new RoomTimelineVideoProcessing({
                status: RoomTimelineVideoProcessingStatus.COMPLETED,
                variants: [
                  new RoomTimelineVideoVariant({
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
