import type { InfiniteData } from '@tanstack/svelte-query';
import type { QueryKey } from '@tanstack/svelte-query';
import type { FollowedThread, FollowedThreadsPage } from '$lib/api-client/threads';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { queryClient } from './client';
import { registerFollowedThreadQueryCache } from './cacheRegistry';

type ThreadQueryConnection = Pick<ServerConnection, 'queryScope'>;
type ThreadViewerState = { hasUnreadReplies?: boolean };

export type FollowedThreadsQueryPage = FollowedThreadsPage & {
  /** Next server offset, preserved even when projection reconciliation filters rows. */
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
  return ['server', serverId, 'session', connection.queryScope, 'threads'] as const;
}

export const threadQueryKeys = {
  followed(serverId: string, connection: ThreadQueryConnection) {
    return [...threadRoot(serverId, connection), 'followed'] as const;
  }
};

export function followedThreadKey(roomId: string, threadRootEventId: string): string {
  return `${roomId}\u0000${threadRootEventId}`;
}

/** Flatten offset pages without rendering a duplicate returned across page boundaries. */
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

/**
 * Scrub threads absent from the authoritative followed-thread projection and
 * reconcile unread state. The caller should refetch once when the projection
 * contains a thread that the loaded snapshot does not yet contain.
 */
export function reconcileFollowedThreadViewerStates(
  data: FollowedThreadsData | undefined,
  states: ReadonlyMap<string, ThreadViewerState>
): { data: FollowedThreadsData | undefined; hasUnknownThreads: boolean } {
  if (!data) return { data, hasUnknownThreads: states.size > 0 };

  const cachedTotalCount = data.pages[0]?.totalCount ?? 0;
  const snapshotComplete = data.pages.length > 0 && !data.pages[data.pages.length - 1]!.hasMore;
  const knownKeys = new Set<string>();
  let changed = false;
  let pages = data.pages.map((page) => {
    const threads = page.threads.flatMap((thread) => {
      const key = followedThreadKey(thread.roomId, thread.threadRootEventId);
      knownKeys.add(key);
      const state = states.get(key);
      if (!state) {
        changed = true;
        return [];
      }
      const hasUnreadReplies = state.hasUnreadReplies ?? false;
      if (thread.hasUnreadReplies === hasUnreadReplies) {
        return [thread];
      }
      changed = true;
      return [{ ...thread, hasUnreadReplies }];
    });
    return threads.length === page.threads.length && !changed ? page : { ...page, threads };
  });
  const hasMissingProjectionThreads = [...states.keys()].some((key) => !knownKeys.has(key));
  const hasUnknownThreads =
    hasMissingProjectionThreads && (snapshotComplete || states.size !== cachedTotalCount);
  pages = pages.map((page, index) => {
    const isLastPage = index === pages.length - 1;
    const hasMore = isLastPage && !hasMissingProjectionThreads ? false : page.hasMore;
    if (page.totalCount === states.size && page.hasMore === hasMore) return page;
    changed = true;
    return { ...page, totalCount: states.size, hasMore };
  });

  return {
    data: changed ? { ...data, pages } : data,
    hasUnknownThreads
  };
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

function resumeFollowedThreadQuery(queryKey: QueryKey): void {
  void queryClient
    .cancelQueries({ queryKey, exact: true }, { revert: false })
    .then(() => queryClient.invalidateQueries({ queryKey, exact: true }));
}

function reconcileFollowedThreadQueries(
  serverId: string,
  states: ReadonlyMap<string, ThreadViewerState>
): void {
  for (const query of followedThreadQueries(serverId)) {
    const current = query.state.data as FollowedThreadsData | undefined;
    const reconciled = reconcileFollowedThreadViewerStates(current, states);
    if (flattenFollowedThreads(reconciled.data).length < flattenFollowedThreads(current).length) {
      void queryClient.cancelQueries({ queryKey: query.queryKey, exact: true });
    }
    if (reconciled.data !== current) queryClient.setQueryData(query.queryKey, reconciled.data);
    if (reconciled.hasUnknownThreads) {
      void queryClient.invalidateQueries({ queryKey: query.queryKey, exact: true });
    }
  }
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

function scrubFollowedThreadMessage(serverId: string, roomId: string, eventId: string): void {
  for (const query of followedThreadQueries(serverId)) {
    let changed = false;
    queryClient.setQueryData<FollowedThreadsData>(query.queryKey, (current) => {
      if (!current) return current;
      const pages = current.pages.map((page) => ({
        ...page,
        threads: page.threads.map((thread) => {
          if (thread.roomId !== roomId) return thread;
          const scrubRoot =
            thread.rootMessage !== null &&
            (thread.threadRootEventId === eventId || thread.rootMessage.id === eventId);
          const scrubLatestReply = thread.latestReply?.id === eventId;
          if (!scrubRoot && !scrubLatestReply) return thread;
          changed = true;
          return {
            ...thread,
            rootMessage: scrubRoot ? null : thread.rootMessage,
            latestReply: scrubLatestReply ? null : thread.latestReply
          };
        })
      }));
      return changed ? { ...current, pages } : current;
    });
    // A pending initial or next-page response may contain the deleted root even
    // when it is not in the committed pages yet. Fence that response, while
    // leaving settled queries for unrelated deletions alone.
    if (changed || query.state.fetchStatus === 'fetching') {
      resumeFollowedThreadQuery(query.queryKey);
    }
  }
}

function updateFollowedThreadQueries(serverId: string, summary: ThreadSummaryUpdate): void {
  for (const query of followedThreadQueries(serverId)) {
    queryClient.setQueryData<FollowedThreadsData>(query.queryKey, (current) =>
      updateFollowedThreadSummary(current, summary)
    );
  }
}

registerFollowedThreadQueryCache({
  reset: resetFollowedThreadQueries,
  refresh: refreshFollowedThreadQueries,
  reconcile: reconcileFollowedThreadQueries,
  scrubRoom: scrubFollowedThreadRoom,
  scrubMessage: scrubFollowedThreadMessage,
  scrubUser: resetFollowedThreadQueries,
  updateSummary: updateFollowedThreadQueries
});
