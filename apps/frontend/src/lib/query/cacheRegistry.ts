import { runResetHandlers } from '$lib/state/server/resetHandlers';
type ServerCacheRemover = (serverId: string) => void;
type AdminUserCacheRemover = (serverId: string, userId: string) => void;
type AdminUserRemovalListener = (serverId: string, userId: string) => void;
type QueryCacheRemovalListener = (serverId: string) => void;
type AdminRoomQueryReconciler = (serverId: string, roomId: string, removed: boolean) => void;
type AdminRoomGroupQueryReconciler = (serverId: string, visibleGroupIds: readonly string[]) => void;
type FollowedThreadViewerState = {
  hasUnreadReplies?: boolean;
};
type FollowedThreadSummary = {
  roomId: string;
  threadRootEventId: string;
  replyCount: number;
  lastReplyAt: string | null;
  hasUnreadReplies?: boolean;
};
type FollowedThreadCache = {
  reset(serverId: string): void;
  refresh(serverId: string): void;
  reconcile(serverId: string, states: ReadonlyMap<string, FollowedThreadViewerState>): void;
  scrubRoom(serverId: string, roomId: string): void;
  scrubMessage(serverId: string, roomId: string, eventId: string): void;
  scrubUser(serverId: string): void;
  updateSummary(serverId: string, summary: FollowedThreadSummary): void;
};
type RoomMemberQueryCache = {
  invalidateRoom(serverId: string, roomId: string): void;
  purgeRoom(serverId: string, roomId: string): void;
  scrubUser(serverId: string, userId: string): void;
};

let removeServerCache: ServerCacheRemover | undefined;
let refreshServerCache: ((serverId: string) => Promise<void>) | undefined;
let removeAdminCache: ServerCacheRemover | undefined;
let refreshAdminCache: ServerCacheRemover | undefined;
let refreshRoleCache: ServerCacheRemover | undefined;
let removeAdminUserCache: AdminUserCacheRemover | undefined;
let reconcileAdminRoomCache: AdminRoomQueryReconciler | undefined;
let reconcileAdminRoomGroupCache: AdminRoomGroupQueryReconciler | undefined;
let followedThreadCache: FollowedThreadCache | undefined;
let roomMemberQueryCache: RoomMemberQueryCache | undefined;
const adminUserRemovalListeners = new Set<AdminUserRemovalListener>();
const queryCacheRemovalListeners = new Set<QueryCacheRemovalListener>();
const serverQueryCacheRemovalListeners = new Set<QueryCacheRemovalListener>();

/** Register the snapshot-query cache without loading it into every route bundle. */
export function registerServerQueryCache(removers: {
  server: ServerCacheRemover;
  /** Reauthorize active snapshots in place and discard inactive private data. */
  refreshServer?: (serverId: string) => Promise<void>;
  admin: ServerCacheRemover;
  refreshAdmin: ServerCacheRemover;
  roles?: ServerCacheRemover;
  adminUser: AdminUserCacheRemover;
  adminRoom: AdminRoomQueryReconciler;
  adminRoomGroups: AdminRoomGroupQueryReconciler;
}): void {
  removeServerCache = removers.server;
  refreshServerCache = removers.refreshServer;
  removeAdminCache = removers.admin;
  refreshAdminCache = removers.refreshAdmin;
  refreshRoleCache = removers.roles;
  removeAdminUserCache = removers.adminUser;
  reconcileAdminRoomCache = removers.adminRoom;
  reconcileAdminRoomGroupCache = removers.adminRoomGroups;
}

/** Refresh role displays without invalidating the viewer's private-data generation. */
export function refreshRegisteredRoleQueries(serverId: string): void {
  refreshRoleCache?.(serverId);
}

/** Register the followed-thread snapshot cache without loading it into the server store bundle. */
export function registerFollowedThreadQueryCache(cache: FollowedThreadCache): void {
  followedThreadCache = cache;
}

/** Register room-member snapshots without loading TanStack Query into the server-store bundle. */
export function registerRoomMemberQueryCache(cache: RoomMemberQueryCache): void {
  roomMemberQueryCache = cache;
}

export function purgeRegisteredRoomMemberQueries(serverId: string, roomId: string): void {
  roomMemberQueryCache?.purgeRoom(serverId, roomId);
}

export function invalidateRegisteredRoomMemberQueries(serverId: string, roomId: string): void {
  roomMemberQueryCache?.invalidateRoom(serverId, roomId);
}

export function scrubRegisteredRoomMemberUser(serverId: string, userId: string): void {
  roomMemberQueryCache?.scrubUser(serverId, userId);
}

export function resetRegisteredFollowedThreadQueries(serverId: string): void {
  followedThreadCache?.reset(serverId);
}

/** Refresh followed-thread feed data after activity changes its latest reply. */
export function refreshRegisteredFollowedThreadQueries(serverId: string): void {
  followedThreadCache?.refresh(serverId);
}

export function reconcileRegisteredFollowedThreadQueries(
  serverId: string,
  states: ReadonlyMap<string, FollowedThreadViewerState>
): void {
  followedThreadCache?.reconcile(serverId, states);
}

export function scrubRegisteredFollowedThreadRoom(serverId: string, roomId: string): void {
  followedThreadCache?.scrubRoom(serverId, roomId);
}

export function scrubRegisteredFollowedThreadMessage(
  serverId: string,
  roomId: string,
  eventId: string
): void {
  followedThreadCache?.scrubMessage(serverId, roomId, eventId);
}

export function scrubRegisteredFollowedThreadUser(serverId: string): void {
  followedThreadCache?.scrubUser(serverId);
}

export function updateRegisteredFollowedThreadSummary(
  serverId: string,
  summary: FollowedThreadSummary
): void {
  followedThreadCache?.updateSummary(serverId, summary);
}

/** Purge private reads and fence mutations at session or protocol recovery boundaries. */
export function removeRegisteredServerQueries(serverId: string): boolean {
  const listenersCleared = runResetHandlers([
    ...[...queryCacheRemovalListeners, ...serverQueryCacheRemovalListeners].map(
      (listener) => () => listener(serverId)
    )
  ]);
  const cacheCleared = runResetHandlers([
    () => removeServerCache?.(serverId)
  ]);
  return listenersCleared && cacheCleared;
}

/** Fence optimistic results, then reauthorize each snapshot without replacing its observer. */
export async function refreshRegisteredServerQueries(serverId: string): Promise<void> {
  const fenced = runResetHandlers(
    [...queryCacheRemovalListeners, ...serverQueryCacheRemovalListeners].map(
      (listener) => () => listener(serverId)
    )
  );
  await refreshServerCache?.(serverId);
  if (!fenced) throw new Error('Permission refresh could not fence every mutation');
}

/** Purge cached admin reads as soon as their authorization may have changed. */
export function removeRegisteredAdminQueries(serverId: string): void {
  for (const listener of queryCacheRemovalListeners) listener(serverId);
  removeAdminCache?.(serverId);
}

/** Refetch mounted admin reads without discarding their stable render geometry. */
export function refreshRegisteredAdminQueries(serverId: string): void {
  runResetHandlers([...queryCacheRemovalListeners].map((listener) => () => listener(serverId)));
  refreshAdminCache?.(serverId);
}

/** Refresh profile snapshots without fencing mutations as an authorization reset would. */
export function refreshRegisteredAdminProfileQueries(serverId: string): void {
  refreshAdminCache?.(serverId);
}

/** Purge admin snapshots that can retain a removed user's private data. */
export function removeRegisteredAdminUserQueries(serverId: string, userId: string): void {
  for (const listener of adminUserRemovalListeners) listener(serverId, userId);
  removeAdminUserCache?.(serverId, userId);
}

/** Reconcile cached room-management snapshots from the process-wide projection owner. */
export function reconcileRegisteredAdminRoomQueries(
  serverId: string,
  roomId: string,
  removed = false
): void {
  reconcileAdminRoomCache?.(serverId, roomId, removed);
}

/** Reconcile cached room-group snapshots from the authoritative visible group replacement. */
export function reconcileRegisteredAdminRoomGroupQueries(
  serverId: string,
  visibleGroupIds: readonly string[]
): void {
  reconcileAdminRoomGroupCache?.(serverId, visibleGroupIds);
}

/** Observe privacy-driven admin-user removal while a detail owner is mounted. */
export function registerAdminUserRemovalListener(listener: AdminUserRemovalListener): () => void {
  adminUserRemovalListeners.add(listener);
  return () => adminUserRemovalListeners.delete(listener);
}

/** Fence late mutations when authentication or admin snapshots are invalidated. */
export function registerQueryCacheRemovalListener(listener: QueryCacheRemovalListener): () => void {
  queryCacheRemovalListeners.add(listener);
  return () => queryCacheRemovalListeners.delete(listener);
}

/** Fence account mutations only when the complete server session is being disposed. */
export function registerServerQueryCacheRemovalListener(
  listener: QueryCacheRemovalListener
): () => void {
  serverQueryCacheRemovalListeners.add(listener);
  return () => serverQueryCacheRemovalListeners.delete(listener);
}
