import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import type { ConnectAPIConfig } from '$lib/api-client/connect';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { Timestamp } from '@bufbuild/protobuf';
import {
  RoomTimelineEvent,
  RoomTimelinePage,
  RoomTimelineCallEvent,
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
  disposeUserStore,
  getUserStore,
  resetUserStoresForTests
} from '$lib/state/server/users.svelte';
import {
  createRoomTimelineAPI,
  roomTimelinePageToEventConnectionPage
} from '$lib/api-client/roomTimeline';
import { fakeServer, mockService, receivedContext, receivedRequest } from '$lib/test-utils';

const messages = mockService(MessageService);
const threads = mockService(ThreadService);
const users = mockService(UserService);

function config(extra: Partial<ConnectAPIConfig> = {}) {
  return fakeServer(
    (router) =>
      router
        .service(MessageService, messages)
        .service(ThreadService, threads)
        .service(UserService, users),
    extra
  );
}

function timelineAPI() {
  return createRoomTimelineAPI(config({ bearerToken: 'remote-token' }));
}

describe('createRoomTimelineAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    users.batchGetUsers.mockReturnValue({ users: [] });
    resetUserStoresForTests();
  });

  it.each(['reset', 'dispose'])(
    'rejects timeline includes after a connection %s',
    async (boundary) => {
      configureApiClientHooks({});
      const store = getUserStore('remote', 'session');
      let finish!: (response: { page: RoomTimelinePage }) => void;
      threads.getThreadEvents.mockImplementation(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          })
      );
      const api = createRoomTimelineAPI(config({ serverId: 'remote', queryScope: 'session' }));
      const pending = api.getThreadEvents({ roomId: 'room', threadRootEventId: 'root', limit: 20 });
      await vi.waitFor(() => expect(threads.getThreadEvents).toHaveBeenCalled());
      if (boundary === 'reset') store.clear();
      else disposeUserStore('remote', 'session');
      finish({
        page: new RoomTimelinePage({
          includes: {
            users: {
              bot: new User({ id: 'bot', login: 'bot' })
            }
          }
        })
      });
      await expect(pending).rejects.toThrow('Response discarded');
      expect(store.size).toBe(0);
      expect(getUserStore('remote', 'session').size).toBe(0);
    }
  );

  it('does not turn a timeline deletion include into a shared user tombstone', async () => {
    threads.getThreadEvents.mockReturnValue({
      page: new RoomTimelinePage({
        includes: {
          users: {
            author: new User({ id: 'author', deleted: true }),
            colleague: new User({ id: 'colleague', login: 'colleague', displayName: 'Colleague' })
          }
        },
        events: [
          new RoomTimelineEvent({
            id: 'message-1',
            actorId: 'author',
            event: {
              case: 'messagePosted',
              value: new RoomMessagePosted({
                message: new Message({ id: 'message-1', roomId: 'room', actorId: 'author' })
              })
            }
          })
        ]
      })
    });
    const store = getUserStore('remote', 'session');
    const api = createRoomTimelineAPI(config({ serverId: 'remote', queryScope: 'session' }));

    const page = await api.getThreadEvents({
      roomId: 'room',
      threadRootEventId: 'root',
      limit: 20
    });

    expect(page.events[0]?.actor).toMatchObject({ id: 'author', deleted: true });
    expect(store.isDeleted('author')).toBe(false);
    expect(store.missing(['author'])).toEqual(['author']);
    expect(store.get('colleague')?.user?.displayName).toBe('Colleague');
  });

  it('sends thread page requests with opaque cursors', async () => {
    threads.getThreadEvents.mockReturnValue({
      page: new RoomTimelinePage({
        startCursor: 'tl:opaque-start',
        endCursor: 'tl:opaque-end',
        hasOlder: false,
        hasNewer: true
      })
    });

    const api = timelineAPI();

    const page = await api.getThreadEvents({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      limit: 50,
      before: 'tl:opaque-before'
    });

    expect(receivedRequest(threads.getThreadEvents)).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      limit: 50,
      cursor: { case: 'before', value: 'tl:opaque-before' }
    });
    expect(page).toMatchObject({
      startCursor: 'tl:opaque-start',
      endCursor: 'tl:opaque-end',
      hasOlder: false,
      hasNewer: true
    });
  });

  it('sends thread-around requests with the anchor event id', async () => {
    threads.getThreadEventsAround.mockReturnValue({
      page: new RoomTimelinePage({ hasOlder: true, hasNewer: true })
    });

    const api = timelineAPI();

    await api.getThreadEventsAround({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      eventId: 'reply-20',
      limit: 50
    });

    expect(receivedRequest(threads.getThreadEventsAround)).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      eventId: 'reply-20',
      limit: 50
    });
  });

  it('gets messages and hydrates their authors', async () => {
    messages.getMessage.mockReturnValue({
      message: new Message({
        id: 'reply-1',
        actorId: 'u1',
        roomId: 'room-1',
        body: 'thread reply',
        threadRootEventId: 'root-1'
      })
    });
    users.batchGetUsers.mockReturnValue({
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

    const api = timelineAPI();

    const message = await api.getMessage({
      roomId: 'room-1',
      eventId: 'reply-1'
    });

    expect(receivedRequest(messages.getMessage)).toMatchObject({
      roomId: 'room-1',
      eventId: 'reply-1'
    });
    expect(receivedRequest(users.batchGetUsers)).toMatchObject({ userIds: ['u1'] });
    expect(message).toMatchObject({
      id: 'reply-1',
      actor: { id: 'u1', displayName: 'Alice' },
      event: { kind: 'messagePosted', body: 'thread reply', threadRootEventId: 'root-1' }
    });
  });

  it('bounds realtime-triggered message reads by the event cursor', async () => {
    messages.getMessage.mockReturnValue({
      message: new Message({ id: 'message-1', actorId: 'u1', roomId: 'room-1' })
    });
    const api = timelineAPI();

    await api.getMessage({
      roomId: 'room-1',
      eventId: 'message-1',
      minimumCursor: 'opaque-event-cursor'
    });

    for (const handler of [messages.getMessage, users.batchGetUsers]) {
      const context = receivedContext(handler);
      expect(context?.requestHeader.get('Chatto-Realtime-Minimum-Cursor')).toBe(
        'opaque-event-cursor'
      );
      expect(context?.requestHeader.get('Authorization')).toBe('Bearer remote-token');
      expect(context?.timeoutMs()).toBeGreaterThan(9_000);
    }
  });

  it.each([undefined, 'opaque-event-cursor'])(
    'fails message hydration without inventing a deleted author (cursor: %s)',
    async (minimumCursor) => {
      messages.getMessage.mockReturnValue({
        message: new Message({ id: 'message-1', actorId: 'u1', roomId: 'room-1' })
      });
      users.batchGetUsers.mockImplementation(() => {
        throw new ConnectError('user projection unavailable', Code.Unavailable);
      });
      const api = timelineAPI();

      await expect(
        api.getMessage({
          roomId: 'room-1',
          eventId: 'message-1',
          minimumCursor
        })
      ).rejects.toMatchObject({ code: Code.Unavailable });
    }
  );
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
                thread: {
                  replyCount: 1,
                  participantPreviewUserIds: ['u2'],
                  participantCount: 1,
                  viewerState: { isFollowing: true, hasUnreadReplies: true }
                },
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
        }),
        new RoomTimelineEvent({
          id: 'call-started-1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:02Z')),
          actorId: 'u1',
          event: {
            case: 'callStarted',
            value: new RoomTimelineCallEvent({ roomId: 'room-1', callId: 'call-1' })
          }
        }),
        new RoomTimelineEvent({
          id: 'call-ended-1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:03Z')),
          actorId: 'u1',
          event: {
            case: 'callEnded',
            value: new RoomTimelineCallEvent({ roomId: 'room-1', callId: 'call-1' })
          }
        })
      ]
    });

    const mapped = roomTimelinePageToEventConnectionPage(page);

    expect(mapped.startCursor).toBe('tl:opaque-start');
    expect(mapped.hasOlder).toBe(true);
    expect(mapped.events).toHaveLength(4);
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
        viewerIsFollowingThread: true,
        viewerHasUnreadThread: true
      }
    });
    expect(mapped.events[1]).toMatchObject({
      id: 'join1',
      event: { kind: 'userJoinedRoom', roomId: 'room-1' }
    });
    expect(mapped.events[2]).toMatchObject({
      id: 'call-started-1',
      event: { kind: 'callStarted', roomId: 'room-1', callId: 'call-1' }
    });
    expect(mapped.events[3]).toMatchObject({
      id: 'call-ended-1',
      event: { kind: 'callEnded', roomId: 'room-1', callId: 'call-1' }
    });
  });
});
