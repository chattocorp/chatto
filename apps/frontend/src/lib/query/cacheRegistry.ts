/**
 * Late-bound access to the TanStack Query caches.
 *
 * The server store must purge and refresh cached snapshot reads at privacy and
 * realtime boundaries, but it is part of every route's initial bundle. The
 * query modules register their cache operations here when they load, so the
 * store can call them without importing TanStack Query. A cache that has not
 * loaded holds no data, so a missing registration is a no-op.
 *
 * Measured on 2026-09-28: importing `query/client`, `query/threads`, and
 * `query/roomMembers` from the store adds about 9.7 KiB gzip to every route and
 * fails the login and overview budgets. Keep this indirection unless those
 * modules become part of the initial bundles anyway.
 */
import { runResetHandlers } from '$lib/state/server/resetHandlers';

/** Snapshot-query operations registered by `query/client`. */
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
