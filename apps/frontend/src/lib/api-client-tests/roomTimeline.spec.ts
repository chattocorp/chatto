import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { Timestamp } from '@bufbuild/protobuf';
import {
  RoomTimelineEvent,
  RoomTimelinePage,
  RoomTimelineRoomEvent,
  RoomMessagePosted
} from '@chatto/api-types/api/v1/room_timeline_pb';
import {
  Message,
  MessageAssetUrl,
  MessageAttachment,
  MessageVideoProcessing,
  MessageVideoProcessingStatus,
  MessageVideoVariant
} from '@chatto/api-types/api/v1/message_types_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import {
  __resetUserSummaryCachesForTests,
  primeUserSummaryCache
} from '$lib/state/userSummaries.svelte';
import {
  createRoomTimelineAPI,
  roomTimelinePageToEventConnectionPage
} from '$lib/api-client/roomTimeline';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  getMessage: vi.fn(),
  batchGetUsers: vi.fn(),
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

describe('createRoomTimelineAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();
    mocks.getMessage.mockReset();
    mocks.batchGetUsers.mockReset();
    mocks.batchGetUsers.mockResolvedValue({ users: [] });
    mocks.getThreadEvents.mockReset();
    mocks.getThreadEventsAround.mockReset();
    __resetUserSummaryCachesForTests();

    configureApiClientHooks({
      onAuthenticationRequired: mocks.handleAuthenticationRequired,

      onUserSummaries: primeUserSummaryCache
    });
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockImplementation((service) => {
      if (service?.typeName === 'chatto.api.v1.UserService') {
        return {
          batchGetUsers: mocks.batchGetUsers
        };
      }
      return {
        getMessage: mocks.getMessage,
        getThreadEvents: mocks.getThreadEvents,
        getThreadEventsAround: mocks.getThreadEventsAround
      };
    });
  });

  it('sends thread page requests with bearer auth and opaque cursors', async () => {
    mocks.getThreadEvents.mockResolvedValue({
      page: new RoomTimelinePage({
        startCursor: 'tl:opaque-start',
        endCursor: 'tl:opaque-end',
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
      before: 'tl:opaque-before'
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
        cursor: { case: 'before', value: 'tl:opaque-before' }
      },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(page).toMatchObject({
      startCursor: 'tl:opaque-start',
      endCursor: 'tl:opaque-end',
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

  it('gets messages with bearer auth', async () => {
    mocks.getMessage.mockResolvedValue({
      message: new Message({
        id: 'reply-1',
        actorId: 'u1',
        roomId: 'room-1',
        body: 'thread reply',
        threadRootEventId: 'root-1'
      })
    });
    mocks.batchGetUsers.mockResolvedValue({
      users: [
        {
          user: {
            id: 'u1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false
          }
        }
      ]
    });

    const api = createRoomTimelineAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    const message = await api.getMessage({
      roomId: 'room-1',
      eventId: 'reply-1'
    });

    expect(mocks.getMessage).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        eventId: 'reply-1'
      },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(mocks.batchGetUsers).toHaveBeenCalledWith(
      { userIds: ['u1'] },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(message).toMatchObject({
      id: 'reply-1',
      actor: { id: 'u1', displayName: 'Alice' },
      event: { kind: 'messagePosted', body: 'thread reply', threadRootEventId: 'root-1' }
    });
  });
});

describe('roomTimelinePageToEventConnectionPage', () => {
  it('maps hydrated protobuf room timeline pages into the message render shape', () => {
    const page = new RoomTimelinePage({
      startCursor: 'tl:opaque-start',
      endCursor: 'tl:opaque-end',
      hasOlder: true,
      hasNewer: false,
      includes: {
        users: {
          u1: new User({
            id: 'u1',
            login: 'alice',
            displayName: 'Alice',
            avatarUrl: '/avatars/u1',
            deleted: false
          }),
          u2: new User({
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
            value: new RoomMessagePosted({
              message: new Message({
                id: 'm1',
                roomId: 'room-1',
                actorId: 'u1',
                createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
                body: 'hello',
                attachments: [
                  new MessageAttachment({
                    id: 'a-video',
                    filename: 'clip.mp4',
                    contentType: 'video/mp4',
                    width: 1280,
                    height: 720,
                    assetUrl: new MessageAssetUrl({
                      url: '/assets/files/a-video',
                      expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                    }),
                    thumbnailAssetUrl: new MessageAssetUrl({
                      url: '/assets/files/a-video/image/960x800/contain',
                      expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                    }),
                    videoProcessing: new MessageVideoProcessing({
                      status: MessageVideoProcessingStatus.COMPLETED,
                      durationMs: 1234n,
                      width: 1280,
                      height: 720,
                      sourceAvailable: true,
                      thumbnailAssetUrl: new MessageAssetUrl({
                        url: '/assets/files/a-thumb',
                        expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                      }),
                      variants: [
                        new MessageVideoVariant({
                          quality: '720p',
                          width: 1280,
                          height: 720,
                          size: 4567n,
                          assetUrl: new MessageAssetUrl({
                            url: '/assets/files/a-variant',
                            expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                          })
                        })
                      ]
                    })
                  })
                ],
                replyCount: 1,
                threadParticipantPreviewUserIds: ['u2'],
                threadParticipantCount: 1,
                viewerIsFollowingThread: true,
                reactions: [
                  {
                    emoji: 'thumbsup',
                    count: 2,
                    hasReacted: true,
                    previewUserIds: ['u1', 'u2']
                  }
                ]
              })
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

    expect(mapped.startCursor).toBe('tl:opaque-start');
    expect(mapped.hasOlder).toBe(true);
    expect(mapped.events).toHaveLength(2);
    expect(mapped.events[0]).toMatchObject({
      id: 'm1',
      createdAt: '2026-06-01T12:00:00.000Z',
      actor: { id: 'u1', displayName: 'Alice', avatarUrl: '/avatars/u1' },
      event: {
        kind: 'messagePosted',
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
      event: { kind: 'userJoinedRoom', roomId: 'room-1' }
    });
  });
});
