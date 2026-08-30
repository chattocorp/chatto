import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { createThreadAPI } from '$lib/api-client/threads';
import { Timestamp } from '@bufbuild/protobuf';
import { Message, ThreadSummary } from '@chatto/api-types/api/v1/message_types_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  listFollowedThreads: vi.fn(),
  followThread: vi.fn(),
  unfollowThread: vi.fn()
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

describe('createThreadAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.listFollowedThreads.mockReset();
    mocks.followThread.mockReset();
    mocks.unfollowThread.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listFollowedThreads: mocks.listFollowedThreads,
      followThread: mocks.followThread,
      unfollowThread: mocks.unfollowThread
    });
  });

  it('lists followed threads with bearer auth', async () => {
    const lastReplyAt = new Date('2025-01-02T03:04:05.000Z');
    mocks.listFollowedThreads.mockResolvedValue({
      threads: [
        {
          room: { id: 'room-1', name: 'general' },
          thread: {
            threadRootEventId: 'root-1',
            replyCount: 2,
            lastReplyAt: { toDate: () => lastReplyAt },
            viewerState: { hasUnreadReplies: true }
          },
          rootMessage: undefined
        }
      ],
      page: { totalCount: 3n, hasMore: true },
      includes: { users: {} }
    });

    const api = createThreadAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });
    const page = await api.listFollowedThreads({ limit: 20, offset: 40 });

    expect(mocks.listFollowedThreads).toHaveBeenCalledWith(
      { page: { limit: 20, offset: 40 } },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(page).toEqual({
      threads: [
        {
          roomId: 'room-1',
          roomName: 'general',
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

  it('passes cancellation through when listing followed threads', async () => {
    mocks.listFollowedThreads.mockResolvedValue({ threads: [], page: {} });
    const signal = new AbortController().signal;
    const api = createThreadAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await api.listFollowedThreads({ limit: 20, offset: 0 }, { signal });

    expect(mocks.listFollowedThreads).toHaveBeenCalledWith(
      { page: { limit: 20, offset: 0 } },
      { headers: undefined, signal }
    );
  });

  it('maps root, latest reply, and participant user includes', async () => {
    mocks.listFollowedThreads.mockResolvedValue({
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

    const api = createThreadAPI({ baseUrl: '/api/connect', bearerToken: null });
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

  it('follows a thread with bearer auth', async () => {
    mocks.followThread.mockResolvedValue({
      following: true,
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: true }
    });

    const api = createThreadAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });
    const result = await api.followThread({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.followThread).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        threadRootEventId: 'root-1'
      },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(result).toEqual({
      following: true,
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: true }
    });
  });

  it('unfollows a thread without auth headers when no token is available', async () => {
    mocks.unfollowThread.mockResolvedValue({
      following: false,
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: false }
    });

    const api = createThreadAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });
    const result = await api.unfollowThread({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });

    expect(mocks.unfollowThread).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        threadRootEventId: 'root-1'
      },
      {
        headers: undefined
      }
    );
    expect(result).toEqual({
      following: false,
      state: { roomId: 'room-1', threadRootEventId: 'root-1', following: false }
    });
  });

  it('marks the server authentication stale on unauthenticated Connect errors', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.followThread.mockRejectedValue(err);

    const api = createThreadAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'expired-token'
    });

    await expect(api.followThread({ roomId: 'room-1', threadRootEventId: 'root-1' })).rejects.toBe(
      err
    );

    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });
});
