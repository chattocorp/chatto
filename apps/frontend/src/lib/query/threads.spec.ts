import { describe, expect, it } from 'vitest';
import type { FollowedThread, FollowedThreadsPage } from '$lib/api-client/threads';
import {
  flattenFollowedThreads,
  followedThreadKey,
  reconcileFollowedThreadViewerStates,
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
  return { pages, pageParams: pages.map((_, index) => index * 20) };
}

describe('followed thread query helpers', () => {
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
    expect(reconciled.data?.pages[0]).toMatchObject({ totalCount: 1, hasMore: false });
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

    expect(reconcileFollowedThreadViewerStates(current, states).hasUnknownThreads).toBe(false);
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
});
