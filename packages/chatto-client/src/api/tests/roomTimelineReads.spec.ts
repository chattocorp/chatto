import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomThreadingMode } from '@chatto/api-types/api/v1/common_pb';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { Message, MessageVideoProcessingStatus } from '@chatto/api-types/api/v1/message_types_pb';
import {
  RoomMessagePosted,
  RoomTimelineCallEvent,
  RoomTimelineEvent,
  RoomTimelinePage,
  RoomTimelineRoomEvent,
  RoomTimelineThreadingModeChangedEvent
} from '@chatto/api-types/api/v1/room_timeline_pb';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { TimelineEventKind } from '../../timeline/timelineEvents.js';
import { VideoProcessingStatus } from '../../timeline/messageAttachments.js';
import { resetUserStoresForTests } from '../../server/users.js';
import {
  createRoomTimelineAPI,
  messagePostedPayload,
  roomTimelinePageToEventConnectionPage
} from '../roomTimeline.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const rooms = mockService(RoomService);
const threads = mockService(ThreadService);
const messages = mockService(MessageService);
const users = mockService(UserService);

function timelineAPI() {
  return createRoomTimelineAPI(
    fakeServer((router) =>
      router
        .service(RoomService, rooms)
        .service(ThreadService, threads)
        .service(MessageService, messages)
        .service(UserService, users)
    )
  );
}

const emptyPage = {
  events: [],
  startCursor: null,
  endCursor: null,
  hasOlder: false,
  hasNewer: false
};

describe('room timeline reads', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    resetUserStoresForTests();
    users.batchGetUsers.mockReturnValue({ users: [] });
  });

  it('pages room events backwards, forwards, and from the latest event', async () => {
    rooms.getRoomEvents.mockReturnValue({});
    const api = timelineAPI();

    await expect(api.getRoomEvents({ roomId: 'R1', limit: 20 })).resolves.toEqual(emptyPage);
    await api.getRoomEvents({ roomId: 'R1', limit: 20, before: 'c1' });
    await api.getRoomEvents({ roomId: 'R1', limit: 20, after: 'c2' });

    expect(receivedRequest(rooms.getRoomEvents, 0)?.cursor).toEqual({ case: undefined });
    expect(receivedRequest(rooms.getRoomEvents, 1)?.cursor).toEqual({
      case: 'before',
      value: 'c1'
    });
    expect(receivedRequest(rooms.getRoomEvents, 2)?.cursor).toEqual({
      case: 'after',
      value: 'c2'
    });
  });

  it('reads the events around an anchor', async () => {
    rooms.getRoomEventsAround.mockReturnValueOnce({});
    rooms.getRoomEventsAround.mockReturnValueOnce({
      page: { startCursor: 's', endCursor: 'e', hasOlder: true }
    });
    threads.getThreadEventsAround.mockReturnValueOnce({});
    const api = timelineAPI();

    await expect(
      api.getRoomEventsAround({ roomId: 'R1', eventId: 'E1', limit: 20 })
    ).resolves.toEqual(emptyPage);
    await expect(
      api.getRoomEventsAround({ roomId: 'R1', eventId: 'E1', limit: 20 })
    ).resolves.toEqual({ ...emptyPage, startCursor: 's', endCursor: 'e', hasOlder: true });
    expect(receivedRequest(rooms.getRoomEventsAround)).toMatchObject({
      roomId: 'R1',
      eventId: 'E1',
      limit: 20
    });
    await expect(
      api.getThreadEventsAround({
        roomId: 'R1',
        threadRootEventId: 'T1',
        eventId: 'E1',
        limit: 20
      })
    ).resolves.toEqual(emptyPage);
  });

  it('pages thread events forwards and returns null for a missing message', async () => {
    threads.getThreadEvents.mockReturnValue({});
    messages.getMessage.mockReturnValue({});
    const api = timelineAPI();

    await api.getThreadEvents({ roomId: 'R1', threadRootEventId: 'T1', limit: 20, after: 'c' });
    expect(receivedRequest(threads.getThreadEvents)?.cursor).toEqual({
      case: 'after',
      value: 'c'
    });
    await expect(api.getMessage({ roomId: 'R1', eventId: 'E1' })).resolves.toBeNull();
  });
});

describe('room timeline mapping', () => {
  it('maps every room and call event kind and drops unknown events', () => {
    const roomEvent = (id: string, event: RoomTimelineEvent['event']): RoomTimelineEvent =>
      new RoomTimelineEvent({ id, actorId: 'U1', event });
    const page = roomTimelinePageToEventConnectionPage(
      new RoomTimelinePage({
        events: [
          roomEvent('1', {
            case: 'callStarted',
            value: new RoomTimelineCallEvent({ roomId: 'R1', callId: 'C1' })
          }),
          roomEvent('2', {
            case: 'callEnded',
            value: new RoomTimelineCallEvent({ roomId: 'R1', callId: 'C1' })
          }),
          roomEvent('3', {
            case: 'roomCreated',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('4', {
            case: 'roomUpdated',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('5', {
            case: 'roomDeleted',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('6', {
            case: 'roomArchived',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('7', {
            case: 'roomUnarchived',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('8', {
            case: 'roomThreadingModeChanged',
            value: new RoomTimelineThreadingModeChangedEvent({
              roomId: 'R1',
              threadingMode: RoomThreadingMode.REQUIRED
            })
          }),
          roomEvent('9', {
            case: 'userJoinedRoom',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('10', {
            case: 'userLeftRoom',
            value: new RoomTimelineRoomEvent({ roomId: 'R1' })
          }),
          roomEvent('11', { case: 'messagePosted', value: new RoomMessagePosted() }),
          roomEvent('12', { case: undefined })
        ]
      })
    );

    expect(page.events.map((event) => event.event.kind)).toEqual([
      TimelineEventKind.CallStarted,
      TimelineEventKind.CallEnded,
      TimelineEventKind.RoomCreated,
      TimelineEventKind.RoomUpdated,
      TimelineEventKind.RoomDeleted,
      TimelineEventKind.RoomArchived,
      TimelineEventKind.RoomUnarchived,
      TimelineEventKind.RoomThreadingModeChanged,
      TimelineEventKind.UserJoinedRoom,
      TimelineEventKind.UserLeftRoom
    ]);
    expect(page.events[1]?.event).toEqual({
      kind: TimelineEventKind.CallEnded,
      roomId: 'R1',
      callId: 'C1'
    });
    expect(page.events[7]?.event).toMatchObject({ threadingMode: RoomThreadingMode.REQUIRED });
    expect(page.events[0]?.actor).toBeNull();
  });

  it('maps video processing states and drops an unknown state', () => {
    const payload = messagePostedPayload(
      new Message({
        id: 'M1',
        attachments: [
          { id: 'a', videoProcessing: { status: MessageVideoProcessingStatus.PROCESSING } },
          { id: 'b', videoProcessing: { status: MessageVideoProcessingStatus.FAILED } },
          { id: 'c', videoProcessing: { status: MessageVideoProcessingStatus.UNSPECIFIED } }
        ]
      }),
      {}
    );
    expect(payload.attachments.map((attachment) => attachment.videoProcessing)).toEqual([
      {
        status: VideoProcessingStatus.Processing,
        durationMs: null,
        width: null,
        height: null,
        sourceAvailable: false,
        reasonCode: null,
        thumbnailAssetUrl: null,
        hlsMasterPlaylistUrl: null,
        variants: []
      },
      expect.objectContaining({ status: VideoProcessingStatus.Failed }),
      null
    ]);
  });

  it('maps a social post link preview with one level of quoted post', () => {
    const post = {
      provider: 'bluesky',
      url: 'https://bsky.example/post/1',
      text: 'Hello',
      author: { displayName: 'Ada', handle: 'ada.example' },
      publishedAt: Timestamp.fromDate(new Date('2026-09-01T00:00:00Z')),
      images: [{ url: 'https://img.example/1.png', alt: 'A picture', width: 10 }],
      externalLink: { url: 'https://example.test', title: 'Example' },
      contentWarning: 'spoilers'
    };
    const payload = messagePostedPayload(
      new Message({
        id: 'M1',
        linkPreview: {
          url: 'https://bsky.example/post/1',
          title: 'Post',
          socialPost: {
            ...post,
            quotedPost: { ...post, text: 'Quoted', quotedPost: { ...post, text: 'Too deep' } }
          }
        }
      }),
      {}
    );

    expect(payload.linkPreview).toMatchObject({
      url: 'https://bsky.example/post/1',
      title: 'Post',
      description: null,
      siteName: null,
      socialPost: {
        provider: 'bluesky',
        author: { displayName: 'Ada', handle: 'ada.example', avatarUrl: null },
        publishedAt: '2026-09-01T00:00:00.000Z',
        images: [{ url: 'https://img.example/1.png', alt: 'A picture', width: 10, height: null }],
        externalLink: {
          url: 'https://example.test',
          title: 'Example',
          description: null,
          imageUrl: null
        },
        contentWarning: 'spoilers',
        quotedPost: { text: 'Quoted', quotedPost: null }
      }
    });

    const bare = messagePostedPayload(
      new Message({ id: 'M2', linkPreview: { url: 'x', socialPost: { provider: 'p' } } }),
      {}
    );
    expect(bare.linkPreview?.socialPost).toMatchObject({
      url: null,
      author: null,
      externalLink: null,
      contentWarning: null,
      publishedAt: null,
      quotedPost: null
    });
  });
});
