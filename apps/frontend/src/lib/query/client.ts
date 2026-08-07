import { Code, ConnectError } from '@connectrpc/connect';
import { QueryClient, type InfiniteData, type QueryKey } from '@tanstack/svelte-query';
import type { RoomBanList } from '$lib/api-client/rooms';
import type { RoleDetails } from '$lib/api-client/roles';
import { registerServerQueryCache } from './cacheRegistry';

const SERVER_QUERY_STALE_TIME_MS = 30_000;
const SERVER_QUERY_GC_TIME_MS = 5 * 60_000;

function retryServerQuery(failureCount: number, error: Error): boolean {
  if (
    error instanceof ConnectError &&
    [Code.InvalidArgument, Code.NotFound, Code.PermissionDenied, Code.Unauthenticated].includes(
      error.code
    )
  ) {
    return false;
  }
  return failureCount < 1;
}

/** Shared in-memory cache for snapshot-style server reads. */
export const queryClient = new QueryClient({
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
  queryClient.removeQueries({ queryKey: serverQueryRoot(serverId) });
}

export function removeAdminQueries(serverId: string): void {
  void queryClient.resetQueries({
    predicate: (query) => {
      const key = query.queryKey;
      return key[0] === 'server' && key[1] === serverId && key[4] === 'admin';
    }
  });
}

export function removeAdminUserQueries(serverId: string, userId: string): void {
  const isAdminUserQuery = (key: QueryKey): boolean =>
    key[0] === 'server' &&
    key[1] === serverId &&
    key[4] === 'admin' &&
    (key[5] === 'members' ||
      (key[5] === 'member' && key[6] === userId) ||
      (key[5] === 'user-permissions' && key[6] === userId));
  const isMemberListQuery = (key: QueryKey): boolean =>
    isAdminUserQuery(key) && key[5] === 'members';
  const isDeletedUserSnapshot = (key: QueryKey): boolean =>
    isAdminUserQuery(key) && (key[5] === 'member' || key[5] === 'user-permissions');

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
  queryClient.setQueriesData<RoleDetails>(
    {
      predicate: (query) => {
        const key = query.queryKey;
        return (
          key[0] === 'server' && key[1] === serverId && key[4] === 'admin' && key[5] === 'role'
        );
      }
    },
    (details) =>
      details ? { ...details, users: details.users.filter((user) => user.id !== userId) } : details
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
  admin: removeAdminQueries,
  adminUser: removeAdminUserQueries,
  adminRoom: reconcileAdminRoomQueries,
  adminRoomGroups: reconcileAdminRoomGroupQueries
});
