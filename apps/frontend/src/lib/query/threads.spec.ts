import { InfiniteQueryObserver } from '@tanstack/svelte-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FollowedThread, FollowedThreadsPage } from '$lib/api-client/threads';
import {
  reconcileRegisteredFollowedThreadQueries,
  scrubRegisteredFollowedThreadMessage,
  scrubRegisteredFollowedThreadRoom,
  scrubRegisteredFollowedThreadUser
} from './cacheRegistry';
import { queryClient } from './client';
import {
  flattenFollowedThreads,
  followedThreadKey,
  reconcileFollowedThreadViewerStates,
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
    threadRootEventId,
    rootMessage: null,
    replyCount: 1,
    lastReplyAt: '2026-08-01T10:00:00.000Z',
    hasUnread: false,
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

  it('scrubs unfollowed threads and reconciles unread state from the projection', () => {
    const current = data({
      threads: [thread('removed'), thread('retained')],
      totalCount: 2,
      hasMore: false
    });
    const states = new Map([[followedThreadKey('room-1', 'retained'), { hasUnread: true }]]);

    const reconciled = reconcileFollowedThreadViewerStates(current, states);

    expect(flattenFollowedThreads(reconciled.data)).toEqual([
      thread('retained', { hasUnread: true })
    ]);
    expect(reconciled.data?.pages[0]).toMatchObject({
      totalCount: 1,
      hasMore: false,
      nextOffset: 2
    });
    expect(reconciled.hasUnknownThreads).toBe(false);
  });

  it('reports projection threads that are missing from the cached snapshot', () => {
    const current = data({ threads: [thread('root-1')], totalCount: 2, hasMore: false });
    const states = new Map([
      [followedThreadKey('room-1', 'root-1'), { hasUnread: false }],
      [followedThreadKey('room-1', 'root-2'), { hasUnread: true }]
    ]);

    expect(reconcileFollowedThreadViewerStates(current, states).hasUnknownThreads).toBe(true);
  });

  it('does not refetch merely because projected threads belong to unloaded pages', () => {
    const current = data({ threads: [thread('root-1')], totalCount: 2, hasMore: true });
    const states = new Map([
      [followedThreadKey('room-1', 'root-1'), { hasUnread: false }],
      [followedThreadKey('room-1', 'root-2'), { hasUnread: true }]
    ]);

    const reconciled = reconcileFollowedThreadViewerStates(current, states);
    expect(reconciled.hasUnknownThreads).toBe(false);
    expect(reconciled.data?.pages[0]?.hasMore).toBe(true);
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
      hasUnread: true
    });
    const result = flattenFollowedThreads(updated)[0];

    expect(result).toMatchObject({ replyCount: 3, hasUnread: true });
    expect(result?.rootMessage?.event).toMatchObject({
      kind: 'messagePosted',
      replyCount: 3,
      lastReplyAt: '2026-08-02T10:00:00.000Z'
    });
  });

  it('reconciles every cached session from the process-wide projection owner', () => {
    const firstKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    const secondKey = threadQueryKeys.followed('origin', { queryScope: 'session-2' });
    const current = data({
      threads: [thread('removed'), thread('retained')],
      totalCount: 2,
      hasMore: false
    });
    queryClient.setQueryData(firstKey, current);
    queryClient.setQueryData(secondKey, current);

    reconcileRegisteredFollowedThreadQueries(
      'origin',
      new Map([[followedThreadKey('room-1', 'retained'), { hasUnread: true }]])
    );

    for (const key of [firstKey, secondKey]) {
      expect(flattenFollowedThreads(queryClient.getQueryData(key))).toEqual([
        thread('retained', { hasUnread: true })
      ]);
    }
  });

  it('scrubs room, message, and user privacy boundaries from retained caches', () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    const rootMessage = {
      id: 'root-2',
      createdAt: '2026-08-01T09:00:00.000Z',
      event: {
        kind: 'messagePosted' as const,
        roomId: 'room-2',
        body: 'Private root',
        attachments: [],
        reactions: [],
        replyCount: 1,
        threadParticipants: []
      }
    };
    queryClient.setQueryData(
      queryKey,
      data({
        threads: [thread('root-1'), thread('root-2', { roomId: 'room-2', rootMessage })],
        totalCount: 2,
        hasMore: false
      })
    );

    scrubRegisteredFollowedThreadMessage('origin', 'room-2', 'root-2');
    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))[1]?.rootMessage).toBeNull();

    scrubRegisteredFollowedThreadRoom('origin', 'room-1');
    expect(flattenFollowedThreads(queryClient.getQueryData(queryKey))).toHaveLength(1);

    scrubRegisteredFollowedThreadUser('origin');
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

    scrubRegisteredFollowedThreadUser('origin');

    expect(flattenFollowedThreads(observed)).toEqual([]);
    await vi.waitFor(() => expect(queryFn).toHaveBeenCalled());
    await vi.waitFor(() =>
      expect(flattenFollowedThreads(observer.getCurrentResult().data)[0]?.threadRootEventId).toBe(
        'replacement'
      )
    );
    unsubscribe();
  });

  it('does not interrupt a query for an unrelated message deletion', async () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    queryClient.setQueryData(
      queryKey,
      data({ threads: [thread('root-1')], totalCount: 1, hasMore: false })
    );
    const cancel = vi.spyOn(queryClient, 'cancelQueries');

    scrubRegisteredFollowedThreadMessage('origin', 'room-2', 'unrelated');

    expect(cancel).not.toHaveBeenCalled();
  });

  it('restarts hydration when a message is deleted during an active fetch', async () => {
    const queryKey = threadQueryKeys.followed('origin', { queryScope: 'session-1' });
    let firstSignal: AbortSignal | undefined;
    const queryFn = vi
      .fn()
      .mockImplementationOnce(
        ({ signal }: { signal: AbortSignal }) =>
          new Promise<never>((_resolve, reject) => {
            firstSignal = signal;
            signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true });
          })
      )
      .mockResolvedValue({
        threads: [thread('safe-root', { rootMessage: null })],
        totalCount: 1,
        hasMore: false,
        nextOffset: 1
      });
    const observer = new InfiniteQueryObserver(queryClient, {
      queryKey,
      queryFn,
      initialPageParam: 0,
      getNextPageParam: (lastPage) => (lastPage.hasMore ? lastPage.nextOffset : undefined)
    });
    const unsubscribe = observer.subscribe(() => undefined);
    await vi.waitFor(() => expect(queryFn).toHaveBeenCalledTimes(1));

    scrubRegisteredFollowedThreadMessage('origin', 'room-1', 'deleted-root');

    await vi.waitFor(() => expect(firstSignal?.aborted).toBe(true));
    await vi.waitFor(() => expect(queryFn).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(flattenFollowedThreads(observer.getCurrentResult().data)[0]?.threadRootEventId).toBe(
        'safe-root'
      )
    );
    unsubscribe();
  });
});
