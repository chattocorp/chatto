import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Timestamp } from '@bufbuild/protobuf';
import {
  RoomTimelineAssetUrl,
  RoomTimelineAttachment,
  RoomTimelineEvent,
  RoomTimelineMessagePosted,
  RoomTimelinePage,
  RoomTimelineRoomEvent,
  RoomTimelineUser,
  RoomTimelineVideoProcessing,
  RoomTimelineVideoProcessingStatus,
  RoomTimelineVideoVariant
} from '$lib/pb/chatto/api/v1/room_timeline_pb';
import { createRoomTimelineAPI, roomTimelinePageToEventConnectionPage } from './roomTimeline';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  getThreadEvents: vi.fn(),
  getThreadEventsAround: vi.fn()
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    handleAuthenticationRequired: mocks.handleAuthenticationRequired
  }
}));

describe('createRoomTimelineAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();
    mocks.getThreadEvents.mockReset();
    mocks.getThreadEventsAround.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getThreadEvents: mocks.getThreadEvents,
      getThreadEventsAround: mocks.getThreadEventsAround
    });
  });

  it('sends thread page requests with bearer auth and opaque cursors', async () => {
    mocks.getThreadEvents.mockResolvedValue({
      page: new RoomTimelinePage({
        startCursor: 'seq:1',
        endCursor: 'seq:2',
        hasOlder: false,
        hasNewer: true
      })
    });

    const api = createRoomTimelineAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    const page = await api.getThreadEvents({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      limit: 50,
      before: 'seq:10'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.getThreadEvents).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        threadRootEventId: 'root-1',
        limit: 50,
        cursor: { case: 'before', value: 'seq:10' }
      },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(page).toMatchObject({
      startCursor: 'seq:1',
      endCursor: 'seq:2',
      hasOlder: false,
      hasNewer: true
    });
  });

  it('sends thread-around requests with the anchor event id', async () => {
    mocks.getThreadEventsAround.mockResolvedValue({
      page: new RoomTimelinePage({ hasOlder: true, hasNewer: true })
    });

    const api = createRoomTimelineAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await api.getThreadEventsAround({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      eventId: 'reply-20',
      limit: 50
    });

    expect(mocks.getThreadEventsAround).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        threadRootEventId: 'root-1',
        eventId: 'reply-20',
        limit: 50
      },
      {
        headers: undefined
      }
    );
  });
});

describe('roomTimelinePageToEventConnectionPage', () => {
  it('maps hydrated protobuf room timeline pages into the message render shape', () => {
    const page = new RoomTimelinePage({
      startCursor: 'seq:10',
      endCursor: 'seq:11',
      hasOlder: true,
      hasNewer: false,
      includes: {
        users: {
          u1: new RoomTimelineUser({
            id: 'u1',
            login: 'alice',
            displayName: 'Alice',
            avatarUrl: '/avatars/u1',
            deleted: false
          }),
          u2: new RoomTimelineUser({
            id: 'u2',
            login: 'bob',
            displayName: 'Bob',
            deleted: false
          })
        }
      },
      events: [
        new RoomTimelineEvent({
          id: 'm1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
          actorId: 'u1',
          event: {
            case: 'messagePosted',
            value: new RoomTimelineMessagePosted({
              roomId: 'room-1',
              body: 'hello',
              attachments: [
                new RoomTimelineAttachment({
                  id: 'a-video',
                  filename: 'clip.mp4',
                  contentType: 'video/mp4',
                  width: 1280,
                  height: 720,
                  assetUrl: new RoomTimelineAssetUrl({
                    url: '/assets/files/a-video',
                    expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                  }),
                  thumbnailAssetUrl: new RoomTimelineAssetUrl({
                    url: '/assets/files/a-video/image/960x800/contain',
                    expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                  }),
                  videoProcessing: new RoomTimelineVideoProcessing({
                    status: RoomTimelineVideoProcessingStatus.COMPLETED,
                    durationMs: 1234n,
                    width: 1280,
                    height: 720,
                    sourceAvailable: true,
                    thumbnailAssetUrl: new RoomTimelineAssetUrl({
                      url: '/assets/files/a-thumb',
                      expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                    }),
                    variants: [
                      new RoomTimelineVideoVariant({
                        quality: '720p',
                        width: 1280,
                        height: 720,
                        size: 4567n,
                        assetUrl: new RoomTimelineAssetUrl({
                          url: '/assets/files/a-variant',
                          expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                        })
                      })
                    ]
                  })
                })
              ],
              replyCount: 1,
              threadParticipantUserIds: ['u2'],
              viewerIsFollowingThread: true,
              reactions: [
                {
                  emoji: 'thumbsup',
                  count: 2,
                  hasReacted: true,
                  userIds: ['u1', 'u2']
                }
              ]
            })
          }
        }),
        new RoomTimelineEvent({
          id: 'join1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:01Z')),
          actorId: 'u2',
          event: {
            case: 'userJoinedRoom',
            value: new RoomTimelineRoomEvent({ roomId: 'room-1' })
          }
        })
      ]
    });

    const mapped = roomTimelinePageToEventConnectionPage(page);

    expect(mapped.startCursor).toBe('seq:10');
    expect(mapped.hasOlder).toBe(true);
    expect(mapped.events).toHaveLength(2);
    expect(mapped.events[0]).toMatchObject({
      id: 'm1',
      createdAt: '2026-06-01T12:00:00.000Z',
      actor: { id: 'u1', displayName: 'Alice', avatarUrl: '/avatars/u1' },
      event: {
        __typename: 'MessagePostedEvent',
        body: 'hello',
        attachments: [
          {
            id: 'a-video',
            filename: 'clip.mp4',
            contentType: 'video/mp4',
            videoProcessing: {
              status: 'COMPLETED',
              durationMs: 1234,
              width: 1280,
              height: 720,
              sourceAvailable: true,
              thumbnailAssetUrl: { url: '/assets/files/a-thumb' },
              variants: [
                {
                  quality: '720p',
                  width: 1280,
                  height: 720,
                  size: 4567,
                  assetUrl: { url: '/assets/files/a-variant' }
                }
              ]
            }
          }
        ],
        reactions: [
          {
            emoji: 'thumbsup',
            count: 2,
            hasReacted: true,
            users: [
              { id: 'u1', displayName: 'Alice' },
              { id: 'u2', displayName: 'Bob' }
            ]
          }
        ],
        threadParticipants: [{ id: 'u2', displayName: 'Bob' }],
        viewerIsFollowingThread: true
      }
    });
    expect(mapped.events[1]).toMatchObject({
      id: 'join1',
      event: { __typename: 'UserJoinedRoomEvent', roomId: 'room-1' }
    });
  });
});
