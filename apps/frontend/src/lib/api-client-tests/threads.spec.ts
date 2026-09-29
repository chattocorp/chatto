import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createThreadAPI } from '$lib/api-client/threads';
import { Timestamp } from '@bufbuild/protobuf';
import { Message, ThreadSummary } from '@chatto/api-types/api/v1/message_types_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { MessageSearchService } from '@chatto/api-types/api/v1/message_search_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';
import {
  MessageSearchScope,
  MessageSearchGroupBy,
  MessageSearchOrder
} from '@chatto/api-types/api/v1/message_search_pb';

const threads = mockService(ThreadService);
const search = mockService(MessageSearchService);

function threadAPI() {
  return createThreadAPI(
    fakeServer((router) =>
      router.service(ThreadService, threads).service(MessageSearchService, search)
    )
  );
}

describe('createThreadAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('searches followed threads with normalized input, paging, and cancellation', async () => {
    search.searchMessages.mockReturnValue({
      results: [],
      threadTotalCount: 25n,
      nextCursor: 'next-page'
    });
    const api = threadAPI();
    const result = await api.listFollowedThreads({
      limit: 20,
      offset: 0,
      cursor: 'previous-page',
      query: '  from:alice  '
    });
    expect(receivedRequest(search.searchMessages)).toMatchObject({
      query: 'from:alice',
      scope: MessageSearchScope.FOLLOWED_THREADS,
      groupBy: MessageSearchGroupBy.THREAD,
      order: MessageSearchOrder.THREAD_ACTIVITY,
      pageSize: 20,
      cursor: 'previous-page'
    });
    expect(threads.listFollowedThreads).not.toHaveBeenCalled();
    expect(result).toEqual({ threads: [], totalCount: 25, hasMore: true, nextCursor: 'next-page' });
    threads.listFollowedThreads.mockReturnValue({ threads: [], page: {} });
    await api.listFollowedThreads({ limit: 20, offset: 0, query: '   ' });
    expect(threads.listFollowedThreads).toHaveBeenCalledOnce();

    await expect(
      api.listFollowedThreads({ limit: 20, offset: 0, query: 'x' }, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('lists followed threads', async () => {
    const lastReplyAt = new Date('2025-01-02T03:04:05.000Z');
    threads.listFollowedThreads.mockReturnValue({
      threads: [
        {
          room: { id: 'room-1', name: 'general' },
          thread: {
            threadRootEventId: 'root-1',
            replyCount: 2,
            lastReplyAt: Timestamp.fromDate(lastReplyAt),
            viewerState: { hasUnreadReplies: true }
          },
          rootMessage: undefined
        }
      ],
      page: { totalCount: 3n, hasMore: true },
      includes: { users: {} }
    });

    const api = threadAPI();
    const page = await api.listFollowedThreads({ limit: 20, offset: 40 });

    expect(receivedRequest(threads.listFollowedThreads)).toMatchObject({
      includeDirectMessageThreads: true,
      unreadOnly: false,
      page: { limit: 20, offset: 40 }
    });
    expect(page).toEqual({
      threads: [
        {
          roomId: 'room-1',
          roomName: 'general',
          isDirectMessage: false,
          directMessageParticipants: [],
          threadRootEventId: 'root-1',
          rootMessage: null,
          latestReply: null,
          replyCount: 2,
          lastReplyAt: '2025-01-02T03:04:05.000Z',
          participants: [],
          participantCount: 0,
          hasUnreadReplies: true
        }
      ],
      totalCount: 3,
      hasMore: true
    });
  });

  it('maps grouped search context while keeping the matching reply separate', async () => {
    search.searchMessages.mockReturnValue({
      results: [
        {
          message: { id: 'matching-reply' },
          relevanceScore: 3,
          threadContext: {
            room: { id: 'room-1', name: 'general' },
            thread: {
              threadRootEventId: 'root-1',
              replyCount: 2,
              viewerState: { isFollowing: true, hasUnreadReplies: true }
            }
          }
        }
      ],
      threadTotalCount: 1n,
      nextCursor: '',
      includes: { users: {} }
    });
    const api = threadAPI();
    const page = await api.listFollowedThreads({ limit: 20, offset: 0, query: 'needle' });
    expect(page.threads).toHaveLength(1);
    expect(page.threads[0]).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      replyCount: 2,
      rootMessage: null,
      hasUnreadReplies: true
    });
    expect(page.nextCursor).toBeNull();
  });

  it('asks the server for unread threads only when requested', async () => {
    threads.listFollowedThreads.mockReturnValue({ threads: [], page: {} });
    const api = threadAPI();

    await api.listFollowedThreads({ limit: 20, offset: 0, unreadOnly: true });

    expect(receivedRequest(threads.listFollowedThreads)).toMatchObject({
      includeDirectMessageThreads: true,
      unreadOnly: true,
      page: { limit: 20, offset: 0 }
    });
  });

  it('passes cancellation through when listing followed threads', async () => {
    await expect(
      threadAPI().listFollowedThreads({ limit: 20, offset: 0 }, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('maps root, latest reply, and participant user includes', async () => {
    threads.listFollowedThreads.mockReturnValue({
      threads: [
        {
          room: { id: 'room-1', name: 'general' },
          thread: {
            threadRootEventId: 'root-1',
            replyCount: 1,
            participantCount: 1,
            viewerState: {}
          },
          rootMessage: new Message({
            id: 'root-1',
            roomId: 'room-1',
            actorId: 'u1',
            createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
            body: 'Root body',
            thread: new ThreadSummary({ participantPreviewUserIds: ['u2'] })
          }),
          latestReply: new Message({
            id: 'reply-1',
            roomId: 'room-1',
            actorId: 'u2',
            createdAt: Timestamp.fromDate(new Date('2026-06-01T12:01:00Z')),
            body: 'Latest body',
            threadRootEventId: 'root-1'
          })
        }
      ],
      page: {},
      includes: {
        users: {
          u1: new User({ id: 'u1', login: 'alice', displayName: 'Alice' }),
          u2: new User({ id: 'u2', login: 'bob', displayName: 'Bob' })
        }
      }
    });

    const api = threadAPI();
    const page = await api.listFollowedThreads({ limit: 20, offset: 0 });

    expect(page.threads[0]).toMatchObject({
      rootMessage: {
        id: 'root-1',
        actor: { id: 'u1', displayName: 'Alice' },
        event: { kind: 'messagePosted', body: 'Root body' }
      },
      latestReply: {
        id: 'reply-1',
        actor: { id: 'u2', displayName: 'Bob' },
        event: { kind: 'messagePosted', body: 'Latest body', threadRootEventId: 'root-1' }
      },
      participants: [{ id: 'u2', displayName: 'Bob' }],
      participantCount: 1
    });
  });

  it('maps direct-message identity and participant includes', async () => {
    threads.listFollowedThreads.mockReturnValue({
      threads: [
        {
          room: { id: 'dm-1', kind: RoomKind.DM },
          thread: { threadRootEventId: 'root-1', viewerState: {} },
          directMessageParticipantUserIds: ['viewer', 'other']
        }
      ],
      page: {},
      includes: {
        users: {
          viewer: new User({ id: 'viewer', login: 'viewer', displayName: 'Viewer' }),
          other: new User({ id: 'other', login: 'other', displayName: 'Other' })
        }
      }
    });

    const api = threadAPI();
    const page = await api.listFollowedThreads({ limit: 20, offset: 0 });

    expect(page.threads[0]).toMatchObject({
      roomId: 'dm-1',
      isDirectMessage: true,
      directMessageParticipants: [
        { id: 'viewer', displayName: 'Viewer' },
        { id: 'other', displayName: 'Other' }
      ]
    });
  });

  it('follows a thread', async () => {
    threads.followThread.mockReturnValue({
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: true }
    });

    const api = threadAPI();
    const result = await api.followThread({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });

    expect(receivedRequest(threads.followThread)).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });
    expect(result).toEqual({
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: true }
    });
  });

  it('unfollows a thread', async () => {
    threads.unfollowThread.mockReturnValue({
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: false }
    });

    const api = threadAPI();
    const result = await api.unfollowThread({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });

    expect(receivedRequest(threads.unfollowThread)).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });
    expect(result).toEqual({
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: false }
    });
  });

  it('propagates Connect errors', async () => {
    threads.followThread.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    await expect(
      threadAPI().followThread({ roomId: 'room-1', threadRootEventId: 'root-1' })
    ).rejects.toMatchObject({ code: Code.Unauthenticated, rawMessage: 'authentication required' });
  });
});
