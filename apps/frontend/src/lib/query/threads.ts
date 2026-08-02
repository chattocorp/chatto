import type { InfiniteData } from '@tanstack/svelte-query';
import type { FollowedThread, FollowedThreadsPage } from '$lib/api-client/threads';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

type ThreadQueryConnection = Pick<ServerConnection, 'queryScope'>;
type ThreadViewerState = { hasUnread?: boolean };

export type FollowedThreadsData = InfiniteData<FollowedThreadsPage, unknown>;

export type ThreadSummaryUpdate = {
  roomId: string;
  threadRootEventId: string;
  replyCount: number;
  lastReplyAt: string | null;
  hasUnread?: boolean;
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
          (update.hasUnread === undefined || thread.hasUnread === update.hasUnread))
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
        hasUnread: update.hasUnread ?? thread.hasUnread
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
      const hasUnread = state.hasUnread ?? false;
      if (thread.hasUnread === hasUnread) return [thread];
      changed = true;
      return [{ ...thread, hasUnread }];
    });
    return threads.length === page.threads.length && !changed ? page : { ...page, threads };
  });
  const hasMissingProjectionThreads = [...states.keys()].some((key) => !knownKeys.has(key));
  const hasUnknownThreads =
    hasMissingProjectionThreads && (snapshotComplete || states.size !== cachedTotalCount);
  pages = pages.map((page, index) => {
    const isLastPage = index === pages.length - 1;
    const hasMore = isLastPage && !hasUnknownThreads ? false : page.hasMore;
    if (page.totalCount === states.size && page.hasMore === hasMore) return page;
    changed = true;
    return { ...page, totalCount: states.size, hasMore };
  });

  return {
    data: changed ? { ...data, pages } : data,
    hasUnknownThreads
  };
}
