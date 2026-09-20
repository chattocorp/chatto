import { Code, ConnectError } from '@connectrpc/connect';
import { QueryCache, QueryClient, type InfiniteData, type QueryKey } from '@tanstack/svelte-query';
import type { RoomBanList } from '$lib/api-client/rooms';
import { registerServerQueryCache } from './cacheRegistry';

const SERVER_QUERY_STALE_TIME_MS = 30_000;
const SERVER_QUERY_GC_TIME_MS = 5 * 60_000;

/** Identify the current permission recheck so superseded reads cannot end it. */
const permissionRefreshes = new Map<string, symbol>();

function retryServerQuery(failureCount: number, error: Error): boolean {
  if (
    error instanceof ConnectError &&
    [
      Code.InvalidArgument,
      Code.NotFound,
      Code.FailedPrecondition,
      Code.PermissionDenied,
      Code.Unauthenticated
    ].includes(error.code)
  ) {
    return false;
  }
  return failureCount < 1;
}

/** Shared in-memory cache for snapshot-style server reads. */
export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error, query) => {
      const [kind, serverId] = query.queryKey;
      if (kind === 'server' && typeof serverId === 'string' &&
        (permissionRefreshes.has(serverId) ||
          (error instanceof ConnectError &&
            [Code.PermissionDenied, Code.NotFound, Code.Unauthenticated].includes(error.code)))) {
        // TanStack normally keeps the last successful response on a refetch
        // error. A failed permission recheck or denied read must clear it.
        query.setState({ data: undefined, dataUpdatedAt: 0 });
      }
    }
  }),
  defaultOptions: {
    queries: {
      staleTime: SERVER_QUERY_STALE_TIME_MS,
      gcTime: SERVER_QUERY_GC_TIME_MS,
      refetchOnWindowFocus: false,
      retry: retryServerQuery
    },
    mutations: {
      retry: false
    }
  }
});

export function serverQueryRoot(serverId: string): QueryKey {
  return ['server', serverId];
}

/** Remove cached private responses when a server session is disposed. */
export function removeServerQueries(serverId: string): void {
  permissionRefreshes.delete(serverId);
  for (const query of queryClient.getQueryCache().findAll({ queryKey: serverQueryRoot(serverId) }))
    query.reset();
  queryClient.removeQueries({ queryKey: serverQueryRoot(serverId) });
}

/** Keep authorized active data visible while replacing it with a fresh response.
 * Cancel old reads, discard inactive snapshots, and clear failed or offline checks.
 */
export async function refreshServerQueries(serverId: string): Promise<void> {
  const generation = Symbol();
  permissionRefreshes.set(serverId, generation);
  const filters = { queryKey: serverQueryRoot(serverId) };
  await queryClient.cancelQueries(filters);
  if (permissionRefreshes.get(serverId) !== generation) return;
  const queries = queryClient.getQueryCache().findAll(filters);
  // Active queries can load inactive row snapshots through fetchQuery. Clear
  // those dependencies first, so fresh loads cannot reuse or lose stale rows.
  for (const query of queries) if (!query.isActive()) query.reset();
  try {
    // Invalidate all keys before any read can reuse a cached dependency. Older
    // reads were cancelled above; share new reads started by other queries.
    await queryClient.invalidateQueries(
      { ...filters, refetchType: 'active' },
      { cancelRefetch: false }
    );
    if (permissionRefreshes.get(serverId) !== generation) return;
    // Invalidation does not wait for offline reads. Hide their unchecked data;
    // TanStack resumes the existing requests when the client reconnects.
    for (const query of queryClient.getQueryCache().findAll(filters)) {
      if (query.state.fetchStatus === 'paused') {
        query.setState({ data: undefined, dataUpdatedAt: 0 });
      }
    }
  } finally {
    if (permissionRefreshes.get(serverId) === generation) permissionRefreshes.delete(serverId);
  }
}

/** Refresh role and member snapshots after a public role event; retain other data. */
export function refreshRoleQueries(serverId: string): void {
  const filters = {
    predicate: (query: { queryKey: QueryKey }) => {
      const key = query.queryKey;
      return (
        key[0] === 'server' &&
        key[1] === serverId &&
        [
          'roles',
          'role',
          'role-members',
          'role-permissions',
          'members',
          'member',
          'permission-tier'
        ].includes(String(key[5]))
      );
    }
  };
  // An event can arrive during the first query load. Cancel that older read
  // before invalidating so it cannot satisfy the refresh with pre-event data.
  void queryClient.cancelQueries(filters).then(() => queryClient.invalidateQueries(filters));
}

export function removeAdminQueries(serverId: string): void {
  void queryClient.resetQueries({
    predicate: (query) => {
      const key = query.queryKey;
      return key[0] === 'server' && key[1] === serverId && key[4] === 'admin';
    }
  });
}

/** Refresh active admin snapshots after effective privilege state changes. */
export function refreshAdminQueries(serverId: string): void {
  const filters = {
    predicate: (query: { queryKey: QueryKey }) => {
      const key = query.queryKey;
      return key[0] === 'server' && key[1] === serverId && key[4] === 'admin';
    }
  };
  // Cancel first loads too: invalidation alone can reuse a pending response
  // from before the privilege change.
  void queryClient
    .cancelQueries(filters)
    .then(() => queryClient.invalidateQueries({ ...filters, refetchType: 'active' }));
}

export function removeAdminUserQueries(serverId: string, userId: string): void {
  const isAdminUserQuery = (key: QueryKey): boolean =>
    key[0] === 'server' &&
    key[1] === serverId &&
    key[4] === 'admin' &&
    (key[5] === 'members' ||
      key[5] === 'role-members' ||
      (key[5] === 'member' && key[6] === userId) ||
      (key[5] === 'user-permissions' && key[6] === userId));
  const isMemberListQuery = (key: QueryKey): boolean =>
    isAdminUserQuery(key) &&
    ((key[5] === 'members' && key[6] !== 'row') || key[5] === 'role-members');
  const isDeletedUserSnapshot = (key: QueryKey): boolean =>
    isAdminUserQuery(key) &&
    (key[5] === 'member' ||
      key[5] === 'user-permissions' ||
      (key[5] === 'members' && key[6] === 'row' && key[7] === userId));

  queryClient.setQueriesData<InfiniteData<RoomBanList, number>>(
    {
      predicate: (query) => {
        const key = query.queryKey;
        return (
          key[0] === 'server' && key[1] === serverId && key[4] === 'admin' && key[5] === 'bans'
        );
      }
    },
    (data) =>
      data
        ? {
            ...data,
            pages: data.pages.map((page) => ({
              ...page,
              bans: page.bans.map((ban) => ({
                ...ban,
                user: ban.userId === userId || ban.user?.id === userId ? null : ban.user,
                moderator:
                  ban.moderatorId === userId || ban.moderator?.id === userId ? null : ban.moderator
              }))
            }))
          }
        : data
  );

  // Cancel older pages before scrubbing; a late response must not restore PII.
  void queryClient.cancelQueries({ predicate: (query) => isMemberListQuery(query.queryKey) });
  queryClient.setQueriesData<{
    pages: Array<{ users: Array<{ id: string }> }>;
    pageParams: unknown[];
  }>(
    {
      predicate: (query) => {
        return isMemberListQuery(query.queryKey);
      }
    },
    (data) =>
      data
        ? {
            ...data,
            pages: data.pages.map((page) => ({
              ...page,
              users: page.users.filter((user) => user.id !== userId)
            }))
          }
        : data
  );
  queryClient.setQueriesData(
    {
      predicate: (query) => {
        return isDeletedUserSnapshot(query.queryKey);
      }
    },
    null
  );
  queryClient.removeQueries({
    predicate: (query) => isDeletedUserSnapshot(query.queryKey)
  });
  void queryClient.invalidateQueries({
    predicate: (query) => isMemberListQuery(query.queryKey)
  });
}

function isAdminQueryForServer(key: QueryKey, serverId: string): boolean {
  return key[0] === 'server' && key[1] === serverId && key[4] === 'admin';
}

function permissionTierScope(key: QueryKey): { roomId?: unknown; groupId?: unknown } | null {
  if (key[5] !== 'permission-tier') return null;
  const scope = key[6];
  return scope && typeof scope === 'object' ? scope : null;
}

/** Keep every cached session's room detail coherent even when its route is not mounted. */
export function reconcileAdminRoomQueries(
  serverId: string,
  roomId: string,
  removed: boolean
): void {
  const isRoomDetail = (key: QueryKey): boolean =>
    isAdminQueryForServer(key, serverId) && key[5] === 'room' && key[6] === roomId;
  const isRoomPermissions = (key: QueryKey): boolean =>
    isAdminQueryForServer(key, serverId) && permissionTierScope(key)?.roomId === roomId;

  if (removed) {
    void queryClient.cancelQueries({ predicate: (query) => isRoomDetail(query.queryKey) });
    void queryClient.cancelQueries({ predicate: (query) => isRoomPermissions(query.queryKey) });
    queryClient.setQueriesData({ predicate: (query) => isRoomDetail(query.queryKey) }, null);
    queryClient.setQueriesData({ predicate: (query) => isRoomPermissions(query.queryKey) }, null);
    queryClient.removeQueries({ predicate: (query) => isRoomPermissions(query.queryKey) });
  }
  void queryClient.invalidateQueries({ predicate: (query) => isRoomDetail(query.queryKey) });
}

/** Invalidate visible groups and purge snapshots no longer present in the viewer projection. */
export function reconcileAdminRoomGroupQueries(
  serverId: string,
  visibleGroupIds: readonly string[]
): void {
  const visible = new Set(visibleGroupIds);
  const isRemovedGroupPermissions = (key: QueryKey): boolean => {
    if (!isAdminQueryForServer(key, serverId)) return false;
    const groupId = permissionTierScope(key)?.groupId;
    return typeof groupId === 'string' && !visible.has(groupId);
  };
  const groupQueries = queryClient.getQueryCache().findAll({
    predicate: (query) => {
      const key = query.queryKey;
      return isAdminQueryForServer(key, serverId) && key[5] === 'room-group';
    }
  });

  for (const query of groupQueries) {
    const groupId = query.queryKey[6];
    if (typeof groupId !== 'string') continue;
    if (visible.has(groupId)) {
      void queryClient.invalidateQueries({ queryKey: query.queryKey, exact: true });
      continue;
    }

    void queryClient.cancelQueries({ queryKey: query.queryKey, exact: true });
    queryClient.setQueryData(query.queryKey, null);
    void queryClient.invalidateQueries({ queryKey: query.queryKey, exact: true });
  }

  // Permission matrices can outlive their detail query, so scrub them independently.
  void queryClient.cancelQueries({
    predicate: (query) => isRemovedGroupPermissions(query.queryKey)
  });
  queryClient.setQueriesData(
    { predicate: (query) => isRemovedGroupPermissions(query.queryKey) },
    null
  );
  queryClient.removeQueries({
    predicate: (query) => isRemovedGroupPermissions(query.queryKey)
  });
}

registerServerQueryCache({
  server: removeServerQueries,
  refreshServer: refreshServerQueries,
  admin: removeAdminQueries,
  refreshAdmin: refreshAdminQueries,
  roles: refreshRoleQueries,
  adminUser: removeAdminUserQueries,
  adminRoom: reconcileAdminRoomQueries,
  adminRoomGroups: reconcileAdminRoomGroupQueries
});
