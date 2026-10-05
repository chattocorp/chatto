import { InfiniteQueryObserver } from '@tanstack/svelte-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FollowedThread, FollowedThreadsPage } from '@chatto/client/api/threads';
import { queryCaches } from './cacheRegistry';
import { queryClient } from './queryClient';
import {
  flattenFollowedThreads,
  nextUnreadFollowedThreadOffset,
  threadQueryKeys,
  updateFollowedThreadSummary,
  type FollowedThreadsData
} from './threads';

function thread(
  threadRootEventId: string,
  overrides: Partial<FollowedThread> = {}
): FollowedThread {
  return {
    roomId: 'room-1',
    roomName: 'general',
    isDirectMessage: false,
    directMessageParticipants: [],
    threadRootEventId,
    rootMessage: null,
    latestReply: null,
    replyCount: 1,
    lastReplyAt: '2026-08-01T10:00:00.000Z',
    participants: [],
    participantCount: 0,
    hasUnreadReplies: false,
    ...overrides
  };
}

function data(...pages: FollowedThreadsPage[]): FollowedThreadsData {
  return {
    pages: pages.map((page, index) => ({ ...page, nextOffset: index * 20 + page.threads.length })),
    pageParams: pages.map((_, index) => index * 20)
  };
}

describe('followed thread query helpers', () => {
  afterEach(() => queryClient.clear());

  it('flattens pages without duplicating a thread returned across page boundaries', () => {
    const first = thread('root-1');
    const duplicate = thread('root-1', { replyCount: 2 });
    const second = thread('root-2');

    expect(
      flattenFollowedThreads(
        data(
          { threads: [first], totalCount: 2, hasMore: true },
          { threads: [duplicate, second], totalCount: 2, hasMore: false }
        )
      )
    ).toEqual([first, second]);
  });

  it('updates both the list summary and renderable root message', () => {
    const current = data({
      threads: [
        thread('root-1', {
          rootMessage: {
            id: 'root-1',
            createdAt: '2026-08-01T09:00:00.000Z',
            event: {
              kind: 'messagePosted',
              roomId: 'room-1',
              body: 'Root message',
              attachments: [],
              reactions: [],
              replyCount: 1,
              lastReplyAt: '2026-08-01T10:00:00.000Z',
              threadParticipants: []
            }
          }
        })
      ],
      totalCount: 1,
      hasMore: false
    });

    const updated = updateFollowedThreadSummary(current, {
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      replyCount: 3,
      lastReplyAt: '2026-08-02T10:00:00.000Z',
      hasUnreadReplies: true
    });
    const result = flattenFollowedThreads(updated)[0];

    expect(result).toMatchObject({
      replyCount: 3,
      hasUnreadReplies: true
    });
    expect(result?.rootMessage?.event).toMatchObject({
      kind: 'messagePosted',
      replyCount: 3,
      lastReplyAt: '2026-08-02T10:00:00.000Z'
    });
  });

  it('does not advance the unread offset for loaded threads that became read', () => {
    const pages = data(
      {
        threads: [
          thread('read-1'),
          thread('unread-1', { hasUnreadReplies: true }),
          thread('read-2')
        ],
        totalCount: 5,
        hasMore: true
      },
      {
        threads: [
          thread('unread-1', { hasUnreadReplies: true }),
          thread('unread-2', { hasUnreadReplies: true })
        ],
        totalCount: 5,
        hasMore: true
      }
    ).pages;

    // The server no longer returns read-1 and read-2 in its unread feed, and
    // the duplicate unread-1 counts once.
    expect(nextUnreadFollowedThreadOffset(pages)).toBe(2);
  });

  it('keeps complete, unread, and search feeds in separate cache entries', () => {
    const connection = { queryScope: 'session-1' };
    const keys = [
      threadQueryKeys.followed('origin', connection),
      threadQueryKeys.followed('origin', connection, { unreadOnly: true }),
      threadQueryKeys.followed('origin', connection, { query: 'unread' })
    ].map((key) => JSON.stringify(key));
    expect(new Set(keys).size).toBe(3);
  });

  it('drops the feed when a retracted message is shown in a cached thread', () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    const latestReply = { id: 'reply-2' } as FollowedThread['latestReply'];
    queryClient.setQueryData(
      queryKey,
      data({
        threads: [thread('root-1'), thread('root-2', { roomId: 'room-2', latestReply })],
        totalCount: 2,
        hasMore: false
      })
    );

    queryCaches.followedThreads!.retractMessage('origin', 'room-2', 'reply-2');

    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))).toEqual([]);
  });

  it('keeps loaded pages for a retraction that no cached thread shows', () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    queryClient.setQueryData(
      queryKey,
      data({ threads: [thread('root-1'), thread('root-2')], totalCount: 2, hasMore: false })
    );

    queryCaches.followedThreads!.retractMessage('origin', 'room-1', 'older-reply');

    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))).toHaveLength(2);
    expect(queryClient.getQueryState(queryKey)?.isInvalidated).toBe(true);
  });

  it('drops the feed for any retraction while a feed read is pending', async () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    queryClient.setQueryData(
      queryKey,
      data({ threads: [thread('root-1')], totalCount: 1, hasMore: false })
    );
    let resolveRead!: (page: FollowedThreadsPage) => void;
    const pending = queryClient.fetchInfiniteQuery({
      queryKey,
      queryFn: () => new Promise<FollowedThreadsPage>((resolve) => (resolveRead = resolve)),
      initialPageParam: 0,
      staleTime: 0
    });
    expect(queryClient.getQueryState(queryKey)?.fetchStatus).toBe('fetching');

    queryCaches.followedThreads!.retractMessage('origin', 'room-9', 'unknown');

    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))).toEqual([]);
    resolveRead({ threads: [], totalCount: 0, hasMore: false });
    await pending.catch(() => undefined);
  });

  it('scrubs room and reset privacy boundaries from retained caches', () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    queryClient.setQueryData(
      queryKey,
      data({
        threads: [thread('root-1'), thread('root-2', { roomId: 'room-2' })],
        totalCount: 2,
        hasMore: false
      })
    );

    queryCaches.followedThreads!.scrubRoom('origin', 'room-1');
    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))).toHaveLength(1);

    queryCaches.followedThreads!.reset('origin');
    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))).toEqual([]);
  });

  it('immediately clears mounted data and refetches after a user privacy boundary', async () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    const queryFn = vi.fn().mockResolvedValue({
      threads: [thread('replacement')],
      totalCount: 1,
      hasMore: false,
      nextOffset: 1
    });
    queryClient.setQueryData(
      queryKey,
      data({ threads: [thread('private')], totalCount: 1, hasMore: false })
    );
    const observer = new InfiniteQueryObserver(queryClient, {
      queryKey,
      queryFn,
      initialPageParam: 0,
      getNextPageParam: (lastPage) => (lastPage.hasMore ? lastPage.nextOffset : undefined)
    });
    let observed = observer.getCurrentResult().data;
    const unsubscribe = observer.subscribe((result) => {
      observed = result.data;
    });

    queryCaches.followedThreads!.reset('origin');

    expect(flattenFollowedThreads(observed)).toEqual([]);
    await vi.waitFor(() => expect(queryFn).toHaveBeenCalled());
    await vi.waitFor(() =>
      expect(flattenFollowedThreads(observer.getCurrentResult().data)[0]?.threadRootEventId).toBe(
        'replacement'
      )
    );
    unsubscribe();
  });
});
