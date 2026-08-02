type ServerCacheRemover = (serverId: string) => void;
type AdminUserCacheRemover = (serverId: string, userId: string) => void;
type AdminUserRemovalListener = (serverId: string, userId: string) => void;
type QueryCacheRemovalListener = (serverId: string) => void;
type AdminRoomQueryReconciler = (serverId: string, roomId: string, removed: boolean) => void;
type AdminRoomGroupQueryReconciler = (serverId: string, visibleGroupIds: readonly string[]) => void;

let removeServerCache: ServerCacheRemover | undefined;
let removeAdminCache: ServerCacheRemover | undefined;
let removeAdminUserCache: AdminUserCacheRemover | undefined;
let reconcileAdminRoomCache: AdminRoomQueryReconciler | undefined;
let reconcileAdminRoomGroupCache: AdminRoomGroupQueryReconciler | undefined;
const adminUserRemovalListeners = new Set<AdminUserRemovalListener>();
const queryCacheRemovalListeners = new Set<QueryCacheRemovalListener>();
const serverQueryCacheRemovalListeners = new Set<QueryCacheRemovalListener>();

/** Register the snapshot-query cache without loading it into every route bundle. */
export function registerServerQueryCache(removers: {
  server: ServerCacheRemover;
  admin: ServerCacheRemover;
  adminUser: AdminUserCacheRemover;
  adminRoom: AdminRoomQueryReconciler;
  adminRoomGroups: AdminRoomGroupQueryReconciler;
}): void {
  removeServerCache = removers.server;
  removeAdminCache = removers.admin;
  removeAdminUserCache = removers.adminUser;
  reconcileAdminRoomCache = removers.adminRoom;
  reconcileAdminRoomGroupCache = removers.adminRoomGroups;
}

/** Purge cached private reads when a server session is disposed. */
export function removeRegisteredServerQueries(serverId: string): void {
  for (const listener of queryCacheRemovalListeners) listener(serverId);
  for (const listener of serverQueryCacheRemovalListeners) listener(serverId);
  removeServerCache?.(serverId);
}

/** Purge cached admin reads as soon as their authorization may have changed. */
export function removeRegisteredAdminQueries(serverId: string): void {
  for (const listener of queryCacheRemovalListeners) listener(serverId);
  removeAdminCache?.(serverId);
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

/** Fence late query mutations when authentication or admin visibility clears cached data. */
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
