import type { QueryKey } from '@tanstack/svelte-query';
import { adminQueryKeys } from './admin';
import type { QueryKeyScope } from './keys';
import { purgeQueries, queryClient } from './queryClient';

/** Refresh role listings and every user matrix whose effective decisions can inherit role rules. */
export function invalidateRolePermissionDependents(scope: QueryKeyScope, roleName?: string): void {
  invalidatePermissionTiers(scope);
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.userPermissionsRoot(scope)
  });
  if (roleName) {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.role(scope, roleName),
      exact: true,
      // The role page owns editable metadata drafts. Mark an active snapshot stale
      // without refetching underneath those drafts; inactive snapshots refresh now.
      refetchType: 'inactive'
    });
  }
}

/** Refresh role listings after role metadata or membership changes. */
export function invalidatePermissionTiers(scope: QueryKeyScope): void {
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.permissionTiers(scope)
  });
  void queryClient.invalidateQueries({
    queryKey: adminQueryKeys.roleCatalog(scope)
  });
}

/** Remove a deleted role snapshot and refresh every cache that can derive from it. */
export function removeDeletedRoleQueries(scope: QueryKeyScope, roleName: string): void {
  const roleDetailsKey = adminQueryKeys.role(scope, roleName);
  purgeQueries({ queryKey: adminQueryKeys.rolePermissions(scope, roleName), exact: true });
  queryClient.setQueryData(roleDetailsKey, null);
  queryClient.removeQueries({ queryKey: roleDetailsKey, exact: true });
  purgeQueries({ queryKey: adminQueryKeys.roleMembers(scope, roleName), exact: true });
  invalidateRolePermissionDependents(scope, roleName);
}

/** Refresh the room and room-group snapshots affected by a layout projection. */
export function invalidateAdminRoomLayoutQueries(
  scope: QueryKeyScope,
  roomId?: string,
  groupId?: string
): void {
  if (roomId) {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.room(scope, roomId),
      exact: true
    });
  }
  if (groupId) {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.roomGroup(scope, groupId),
      exact: true
    });
  }
}

/**
 * Purge a detail snapshot and its permission matrix, then read the detail
 * again. The new read shows whether the viewer still has access.
 */
function purgeForReauthorization(queryKey: QueryKey, permissionsKey: QueryKey): void {
  purgeQueries({ queryKey: permissionsKey, exact: true });
  void queryClient.cancelQueries({ queryKey, exact: true });
  queryClient.setQueryData(queryKey, null);
  void queryClient.invalidateQueries({ queryKey, exact: true });
}

/** Purge a room snapshot before attempting any projected re-authorization read. */
export function purgeAdminRoomQuery(scope: QueryKeyScope, roomId: string): void {
  purgeForReauthorization(
    adminQueryKeys.room(scope, roomId),
    adminQueryKeys.permissionTier(scope, roomId, null)
  );
}

/** Purge a room-group snapshot before attempting any projected re-authorization read. */
export function purgeAdminRoomGroupQuery(scope: QueryKeyScope, groupId: string): void {
  purgeForReauthorization(
    adminQueryKeys.roomGroup(scope, groupId),
    adminQueryKeys.permissionTier(scope, null, groupId)
  );
}
