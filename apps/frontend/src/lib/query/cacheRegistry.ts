/**
 * Late-bound access to the TanStack Query caches.
 *
 * Server stores cross privacy and realtime boundaries on every route, but the
 * query modules load only with the pages that use them. `connectQueryCaches`
 * subscribes to each store's boundary events and calls the cache operations
 * that the query modules register here when they load. A cache that has not
 * loaded holds no data, so a missing registration is a no-op.
 *
 * Measured on 2026-09-28: importing `query/queryClient`, `query/threads`, and
 * `query/roomMembers` at startup adds about 9.7 KiB gzip to every route and
 * fails the login and overview budgets. Keep this indirection unless those
 * modules become part of the initial bundles anyway.
 */
import { runResetHandlers } from '@chatto/client/server/resetHandlers';
import type { RealtimeProjectionUpdate } from '@chatto/client/realtime/eventBus';
import type { ServerStateStore } from '@chatto/client/server/store';

/** Snapshot-query operations registered by `query/queryClient`. */
export type ServerQueryCache = {
  /** Remove every cached read of a server. */
  remove(serverId: string): void;
  /** Reauthorize active snapshots in place and discard inactive private data. */
  refresh(serverId: string): Promise<void>;
  /** Remove cached admin reads. */
  removeAdmin(serverId: string): void;
  /** Refetch mounted admin reads without discarding their render geometry. */
  refreshAdmin(serverId: string): void;
  /** Refresh role displays without invalidating the private-data generation. */
  refreshRoles(serverId: string): void;
  /** Remove admin snapshots that can retain a removed user's private data. */
  removeAdminUser(serverId: string, userId: string): void;
  /** Reconcile cached room groups with the visible groups of the projection. */
  reconcileAdminRoomGroups(serverId: string, visibleGroupIds: readonly string[]): void;
};

/** Followed-thread feed operations registered by `query/threads`. */
export type FollowedThreadCache = {
  /** Drop the cached feed and refetch it. */
  reset(serverId: string): void;
  /** Refetch the feed after activity changes a latest reply. */
  refresh(serverId: string): void;
  /**
   * Drop the feed when a cached or pending page can show a retracted message;
   * otherwise refetch it in place.
   */
  retractMessage(serverId: string, roomId: string, eventId: string): void;
  /** Remove threads of a room that the viewer can no longer read. */
  scrubRoom(serverId: string, roomId: string): void;
};

/** Room-member snapshot operations registered by `query/roomMembers`. */
export type RoomMemberQueryCache = {
  /** Remove every session's member snapshots of a room the viewer lost. */
  purgeRoom(serverId: string, roomId: string): void;
  /** Remove a deleted user from every cached member and eligible-user list. */
  scrubUser(serverId: string, userId: string): void;
};

/** The registered caches. Each query module sets its entry when it loads. */
export const queryCaches: {
  server?: ServerQueryCache;
  followedThreads?: FollowedThreadCache;
  roomMembers?: RoomMemberQueryCache;
} = {};

type ServerListener = (serverId: string) => void;
type AdminUserRemovalListener = (serverId: string, userId: string) => void;

const adminUserRemovalListeners = new Set<AdminUserRemovalListener>();
const queryCacheRemovalListeners = new Set<ServerListener>();
const serverQueryCacheRemovalListeners = new Set<ServerListener>();

/** Reset handlers that notify each listener of a server. */
function notify(listeners: Iterable<ServerListener>, serverId: string): (() => void)[] {
  return [...listeners].map((listener) => () => listener(serverId));
}

/** Purge private reads and fence mutations at session or protocol recovery boundaries. */
export function removeRegisteredServerQueries(serverId: string): boolean {
  const listenersCleared = runResetHandlers(
    notify([...queryCacheRemovalListeners, ...serverQueryCacheRemovalListeners], serverId)
  );
  const cacheCleared = runResetHandlers([() => queryCaches.server?.remove(serverId)]);
  return listenersCleared && cacheCleared;
}

/** Fence optimistic results, then reauthorize each snapshot without replacing its observer. */
export async function refreshRegisteredServerQueries(serverId: string): Promise<void> {
  const fenced = runResetHandlers(
    notify([...queryCacheRemovalListeners, ...serverQueryCacheRemovalListeners], serverId)
  );
  await queryCaches.server?.refresh(serverId);
  if (!fenced) throw new Error('Permission refresh could not fence every mutation');
}

/** Purge cached admin reads as soon as their authorization may have changed. */
export function removeRegisteredAdminQueries(serverId: string): void {
  for (const listener of queryCacheRemovalListeners) listener(serverId);
  queryCaches.server?.removeAdmin(serverId);
}

/** Refetch mounted admin reads without discarding their stable render geometry. */
export function refreshRegisteredAdminQueries(serverId: string): void {
  runResetHandlers(notify(queryCacheRemovalListeners, serverId));
  queryCaches.server?.refreshAdmin(serverId);
}

/** Purge admin snapshots that can retain a removed user's private data. */
export function removeRegisteredAdminUserQueries(serverId: string, userId: string): void {
  for (const listener of adminUserRemovalListeners) listener(serverId, userId);
  queryCaches.server?.removeAdminUser(serverId, userId);
}

/** Observe privacy-driven admin-user removal while a detail owner is mounted. */
export function registerAdminUserRemovalListener(listener: AdminUserRemovalListener): () => void {
  adminUserRemovalListeners.add(listener);
  return () => adminUserRemovalListeners.delete(listener);
}

/** Fence late mutations when authentication or admin snapshots are invalidated. */
export function registerQueryCacheRemovalListener(listener: ServerListener): () => void {
  queryCacheRemovalListeners.add(listener);
  return () => queryCacheRemovalListeners.delete(listener);
}

/** Fence account mutations only when the complete server session is being disposed. */
export function registerServerQueryCacheRemovalListener(listener: ServerListener): () => void {
  serverQueryCacheRemovalListeners.add(listener);
  return () => serverQueryCacheRemovalListeners.delete(listener);
}

/**
 * Keep the cached reads of one server store within its privacy and
 * authorization boundaries, and refresh them after realtime changes. The
 * store's own events end at its disposal; the returned function stops earlier.
 */
export function connectQueryCaches(store: ServerStateStore): () => void {
  const serverId = store.serverId;
  const stops = [
    store.onReset(({ privacy, retainView }) => {
      if (privacy && !removeRegisteredServerQueries(serverId)) {
        throw new Error('Query cleanup incomplete');
      }
      if (retainView) return;
      queryCaches.followedThreads?.reset(serverId);
      refreshRegisteredAdminQueries(serverId);
    }),
    store.onSessionEnded(() => {
      if (!removeRegisteredServerQueries(serverId)) throw new Error('Query cleanup incomplete');
    }),
    store.onRoomAccessLost(({ roomId, removed }) => {
      queryCaches.followedThreads?.scrubRoom(serverId, roomId);
      if (removed) queryCaches.roomMembers?.purgeRoom(serverId, roomId);
    }),
    store.onUserDeleted((userId) => {
      queryCaches.followedThreads?.reset(serverId);
      queryCaches.roomMembers?.scrubUser(serverId, userId);
      removeRegisteredAdminUserQueries(serverId, userId);
    }),
    store.onAuthorityChanged(({ lost }) => {
      if (lost) removeRegisteredAdminQueries(serverId);
      else refreshRegisteredAdminQueries(serverId);
    }),
    store.onPermissionsChanged(() => refreshRegisteredServerQueries(serverId)),
    store.onUpdate((update) => refreshAfterUpdate(serverId, update)),
    store.onDispose(() => {
      removeRegisteredServerQueries(serverId);
    })
  ];
  return () => {
    for (const stop of stops) stop();
  };
}

/** Refresh the cached reads that a realtime change can make stale. */
function refreshAfterUpdate(serverId: string, update: RealtimeProjectionUpdate): void {
  if (update.resource?.case === 'roomGroups') {
    queryCaches.server?.reconcileAdminRoomGroups(
      serverId,
      update.resource.value.groups.map((group) => group.id)
    );
  }
  const payload = update.event?.event;
  switch (payload?.case) {
    case 'roleAssigned':
    case 'roleRevoked':
    case 'roleDeleted':
    case 'roleCreated':
    case 'roleUpdated':
    case 'rolesReordered':
    case 'rolePermissionsChanged':
      queryCaches.server?.refreshRoles(serverId);
      return;
    case 'userProfileChanged':
    case 'userAccountCreated':
      // Admin rows have a separate private cache; public profile hydration
      // cannot update its email, permission, or search snapshots.
      queryCaches.server?.refreshAdmin(serverId);
      return;
    case 'messagePosted':
      if (payload.value.threadRootEventId) queryCaches.followedThreads?.refresh(serverId);
      return;
    case 'messageEdited':
    case 'threadViewerStateChanged':
      queryCaches.followedThreads?.refresh(serverId);
      return;
    case 'messageRetracted':
      queryCaches.followedThreads?.retractMessage(
        serverId,
        payload.value.roomId,
        payload.value.messageEventId
      );
      return;
  }
}
