import type { InfiniteData } from '@tanstack/svelte-query';
import type { QueryKey } from '@tanstack/svelte-query';
import type { FollowedThread, FollowedThreadsPage } from '@chatto/client/api/threads';
import type { ServerConnection } from '@chatto/client/server/serverConnection';
import { serverSessionQueryRoot } from './keys';
import { queryClient } from './queryClient';
import { queryCaches } from './cacheRegistry';

type ThreadQueryConnection = Pick<ServerConnection, 'queryScope'>;

export type FollowedThreadsQueryPage = FollowedThreadsPage & {
  /** Next list offset, preserved across reconciliation; search uses nextCursor. */
  nextOffset: number;
};

export type FollowedThreadsData = InfiniteData<FollowedThreadsQueryPage, unknown>;

export type ThreadSummaryUpdate = {
  roomId: string;
  threadRootEventId: string;
  replyCount: number;
  lastReplyAt: string | null;
  hasUnreadReplies?: boolean;
};

function threadRoot(serverId: string, connection: ThreadQueryConnection) {
  return [...serverSessionQueryRoot(serverId, connection), 'threads'] as const;
}

export const threadQueryKeys = {
  /**
   * Key for the followed-thread feed. Pass no filter for the complete feed; any
   * segment after the base key marks a filtered feed whose totals do not
   * describe the complete follow projection.
   */
  followed(
    serverId: string,
    connection: ThreadQueryConnection,
    filter: { query?: string; unreadOnly?: boolean } = {}
  ) {
    const base = [...threadRoot(serverId, connection), 'followed'] as const;
    if (filter.query) return [...base, 'search', filter.query] as const;
    if (filter.unreadOnly) return [...base, 'unread'] as const;
    return base;
  }
};

export function followedThreadKey(roomId: string, threadRootEventId: string): string {
  return `${roomId}\u0000${threadRootEventId}`;
}

/** Flatten live pages without rendering a duplicate returned across page boundaries. */
export function flattenFollowedThreads(data: FollowedThreadsData | undefined): FollowedThread[] {
  const seen = new Set<string>();
  return (data?.pages ?? []).flatMap((page) =>
    page.threads.filter((thread) => {
      const key = followedThreadKey(thread.roomId, thread.threadRootEventId);
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
  );
}

/**
 * Return the next offset for a server-filtered unread feed. The server removes
 * a thread from that feed when the viewer reads it, so a loaded thread that is
 * no longer unread must not advance the offset.
 */
export function nextUnreadFollowedThreadOffset(pages: readonly FollowedThreadsQueryPage[]): number {
  return flattenFollowedThreads({ pages: [...pages], pageParams: [] }).filter(
    (thread) => thread.hasUnreadReplies
  ).length;
}

/** Apply the latest projected root-message summary to every cached page. */
export function updateFollowedThreadSummary(
  data: FollowedThreadsData | undefined,
  update: ThreadSummaryUpdate
): FollowedThreadsData | undefined {
  if (!data) return data;

  let changed = false;
  const pages = data.pages.map((page) => ({
    ...page,
    threads: page.threads.map((thread) => {
      if (
        thread.roomId !== update.roomId ||
        thread.threadRootEventId !== update.threadRootEventId ||
        (thread.replyCount === update.replyCount &&
          (update.hasUnreadReplies === undefined ||
            thread.hasUnreadReplies === update.hasUnreadReplies))
      ) {
        return thread;
      }

      changed = true;
      const rootMessage = thread.rootMessage;
      return {
        ...thread,
        rootMessage:
          rootMessage?.event?.kind === 'messagePosted'
            ? {
                ...rootMessage,
                event: {
                  ...rootMessage.event,
                  replyCount: update.replyCount,
                  lastReplyAt: update.lastReplyAt ?? rootMessage.event.lastReplyAt
                }
              }
            : rootMessage,
        replyCount: update.replyCount,
        lastReplyAt: update.lastReplyAt ?? thread.lastReplyAt,
        hasUnreadReplies: update.hasUnreadReplies ?? thread.hasUnreadReplies
      };
    })
  }));

  return changed ? { ...data, pages } : data;
}

function isFollowedThreadQuery(key: QueryKey, serverId: string): boolean {
  return (
    key[0] === 'server' &&
    key[1] === serverId &&
    key[2] === 'session' &&
    key[4] === 'threads' &&
    key[5] === 'followed'
  );
}

function followedThreadQueries(serverId: string) {
  return queryClient.getQueryCache().findAll({
    predicate: (query) => isFollowedThreadQuery(query.queryKey, serverId)
  });
}

function resetFollowedThreadQueries(serverId: string): void {
  const queries = followedThreadQueries(serverId);
  for (const query of queries) {
    queryClient.setQueryData<FollowedThreadsData>(query.queryKey, {
      pages: [],
      pageParams: []
    });
  }

  void queryClient
    .cancelQueries(
      { predicate: (query) => isFollowedThreadQuery(query.queryKey, serverId) },
      { revert: false }
    )
    .then(() =>
      queryClient.resetQueries({
        predicate: (query) => isFollowedThreadQuery(query.queryKey, serverId)
      })
    );
}

function refreshFollowedThreadQueries(serverId: string): void {
  for (const query of followedThreadQueries(serverId)) {
    void queryClient.invalidateQueries({ queryKey: query.queryKey, exact: true });
  }
}

/**
 * Handle a retracted message. When a cached page shows it, or a pending read can
 * return it, drop the feed so the text disappears at once. Otherwise refetch in
 * place, which keeps the loaded pages.
 */
function retractFollowedThreadMessage(serverId: string, roomId: string, eventId: string): void {
  const shown = followedThreadQueries(serverId).some(
    (query) =>
      query.state.fetchStatus === 'fetching' ||
      flattenFollowedThreads(query.state.data as FollowedThreadsData | undefined).some(
        (thread) =>
          thread.roomId === roomId &&
          (thread.threadRootEventId === eventId ||
            thread.rootMessage?.id === eventId ||
            thread.latestReply?.id === eventId)
      )
  );
  if (shown) resetFollowedThreadQueries(serverId);
  else refreshFollowedThreadQueries(serverId);
}

function resumeFollowedThreadQuery(queryKey: QueryKey): void {
  void queryClient
    .cancelQueries({ queryKey, exact: true }, { revert: false })
    .then(() => queryClient.invalidateQueries({ queryKey, exact: true }));
}

function scrubFollowedThreadRoom(serverId: string, roomId: string): void {
  for (const query of followedThreadQueries(serverId)) {
    queryClient.setQueryData<FollowedThreadsData>(query.queryKey, (current) => {
      if (!current) return current;
      const removed = new Set<string>();
      const pages = current.pages.map((page) => ({
        ...page,
        threads: page.threads.filter((thread) => {
          if (thread.roomId !== roomId) return true;
          removed.add(followedThreadKey(thread.roomId, thread.threadRootEventId));
          return false;
        })
      }));
      if (removed.size === 0) return current;
      return {
        ...current,
        pages: pages.map((page) => ({
          ...page,
          totalCount: Math.max(0, page.totalCount - removed.size)
        }))
      };
    });
    // Access loss is a privacy boundary even while a page is being hydrated.
    resumeFollowedThreadQuery(query.queryKey);
  }
}

queryCaches.followedThreads = {
  reset: resetFollowedThreadQueries,
  refresh: refreshFollowedThreadQueries,
  retractMessage: retractFollowedThreadMessage,
  scrubRoom: scrubFollowedThreadRoom
};
