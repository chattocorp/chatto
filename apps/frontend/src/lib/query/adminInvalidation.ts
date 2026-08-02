import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { adminQueryKeys } from './admin';
import { queryClient } from './client';

type AdminQueryConnection = Pick<ServerConnection, 'queryScope'>;

/** Refresh role listings and every user matrix whose effective decisions can inherit role rules. */
export function invalidateRolePermissionDependents(
  serverId: string,
  connection: AdminQueryConnection
): void {
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.permissionTiers(serverId, connection)
  });
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.userPermissionsRoot(serverId, connection)
  });
}

/** Refresh role listings after role metadata or membership changes. */
export function invalidatePermissionTiers(
  serverId: string,
  connection: AdminQueryConnection
): void {
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.permissionTiers(serverId, connection)
  });
}

/** Remove a deleted role snapshot and refresh every cache that can derive from it. */
export function removeDeletedRoleQueries(
  serverId: string,
  connection: AdminQueryConnection,
  roleName: string
): void {
  const roleKey = adminQueryKeys.rolePermissions(serverId, connection, roleName);
  queryClient.setQueryData(roleKey, null);
  queryClient.removeQueries({ queryKey: roleKey, exact: true });
  invalidateRolePermissionDependents(serverId, connection);
}
