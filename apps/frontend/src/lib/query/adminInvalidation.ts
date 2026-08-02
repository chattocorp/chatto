import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { adminQueryKeys } from './admin';
import { queryClient } from './client';

type AdminQueryConnection = Pick<ServerConnection, 'queryScope'>;

/** Refresh role listings and every user matrix whose effective decisions can inherit role rules. */
export function invalidateRolePermissionDependents(
  serverId: string,
  connection: AdminQueryConnection,
  roleName?: string
): void {
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.permissionTiers(serverId, connection)
  });
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.userPermissionsRoot(serverId, connection)
  });
  if (roleName) {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.role(serverId, connection, roleName),
      exact: true,
      // The role page owns editable metadata drafts. Mark an active snapshot stale
      // without refetching underneath those drafts; inactive snapshots refresh now.
      refetchType: 'inactive'
    });
  }
}

/** Refresh role listings after role metadata or membership changes. */
export function invalidatePermissionTiers(
  serverId: string,
  connection: AdminQueryConnection
): void {
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.permissionTiers(serverId, connection)
  });
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.roleCatalog(serverId, connection)
  });
}

/** Remove a deleted role snapshot and refresh every cache that can derive from it. */
export function removeDeletedRoleQueries(
  serverId: string,
  connection: AdminQueryConnection,
  roleName: string
): void {
  const roleKey = adminQueryKeys.rolePermissions(serverId, connection, roleName);
  const roleDetailsKey = adminQueryKeys.role(serverId, connection, roleName);
  queryClient.setQueryData(roleKey, null);
  queryClient.setQueryData(roleDetailsKey, null);
  queryClient.removeQueries({ queryKey: roleKey, exact: true });
  queryClient.removeQueries({ queryKey: roleDetailsKey, exact: true });
  invalidateRolePermissionDependents(serverId, connection, roleName);
}

/** Refresh the room and room-group snapshots affected by a layout projection. */
export function invalidateAdminRoomLayoutQueries(
  serverId: string,
  connection: AdminQueryConnection,
  roomId?: string,
  groupId?: string
): void {
  if (roomId) {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.room(serverId, connection, roomId),
      exact: true
    });
  }
  if (groupId) {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.roomGroup(serverId, connection, groupId),
      exact: true
    });
  }
}

/** Purge a room snapshot before attempting any projected re-authorization read. */
export function purgeAdminRoomQuery(
  serverId: string,
  connection: AdminQueryConnection,
  roomId: string
): void {
  const queryKey = adminQueryKeys.room(serverId, connection, roomId);
  const permissionsKey = adminQueryKeys.permissionTier(serverId, connection, roomId, null);
  void queryClient.cancelQueries({ queryKey, exact: true });
  void queryClient.cancelQueries({ queryKey: permissionsKey, exact: true });
  queryClient.setQueryData(queryKey, null);
  queryClient.setQueryData(permissionsKey, null);
  queryClient.removeQueries({ queryKey: permissionsKey, exact: true });
  void queryClient.invalidateQueries({ queryKey, exact: true });
}

/** Purge a room-group snapshot before attempting any projected re-authorization read. */
export function purgeAdminRoomGroupQuery(
  serverId: string,
  connection: AdminQueryConnection,
  groupId: string
): void {
  const queryKey = adminQueryKeys.roomGroup(serverId, connection, groupId);
  const permissionsKey = adminQueryKeys.permissionTier(serverId, connection, null, groupId);
  void queryClient.cancelQueries({ queryKey, exact: true });
  void queryClient.cancelQueries({ queryKey: permissionsKey, exact: true });
  queryClient.setQueryData(queryKey, null);
  queryClient.setQueryData(permissionsKey, null);
  queryClient.removeQueries({ queryKey: permissionsKey, exact: true });
  void queryClient.invalidateQueries({ queryKey, exact: true });
}
