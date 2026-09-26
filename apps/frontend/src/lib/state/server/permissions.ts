import type { ViewerCapabilities, ViewerState } from '$lib/api-client/viewer';

/**
 * What the viewer may do on one server. The server store derives it from the
 * viewer projection. The server enforces every permission; these flags only
 * decide which UI the client shows.
 */
export type ServerPermissions = ViewerCapabilities & {
  /** False until the store has the viewer projection for the accepted account. */
  loaded: boolean;
  /** Effective server-wide `server.manage`. */
  canManageServer: boolean;
  /** Effective server-wide `server.manage-neighbors`. */
  canManageNeighbors: boolean;
  /** Effective server-wide `room.manage`: the server's room layout editor. */
  canManageRooms: boolean;
  /** Effective server-wide `room.remove-member`: list and lift room suspensions. */
  canModerateRooms: boolean;
  /** Effective server-wide `bot.create`. */
  canCreateBots: boolean;
  /** Effective server-wide `bot.manage`. */
  canManageBots: boolean;
};

/** Permissions before the viewer projection loads, or after it is cleared. */
export const NO_SERVER_PERMISSIONS: ServerPermissions = Object.freeze({
  loaded: false,
  canViewAdmin: false,
  canStartDMs: false,
  canAdminViewUsers: false,
  canAdminManageAccounts: false,
  canAssignRoles: false,
  canAdminViewRoles: false,
  canAdminManageRoles: false,
  canAdminViewSystem: false,
  canAdminViewAudit: false,
  canManageUserPermissions: false,
  canManageInvites: false,
  canManageServer: false,
  canManageNeighbors: false,
  canManageRooms: false,
  canModerateRooms: false,
  canCreateBots: false,
  canManageBots: false
});

/** Map a loaded viewer projection to the permissions that the UI checks. */
export function serverPermissionsFromViewer(viewer: ViewerState): ServerPermissions {
  const can = (permission: string) => viewer.viewerPermissions[permission] ?? false;
  return {
    loaded: true,
    canViewAdmin: viewer.canViewAdmin,
    canStartDMs: viewer.canStartDMs,
    canAdminViewUsers: viewer.canAdminViewUsers,
    canAdminManageAccounts: viewer.canAdminManageAccounts,
    canAssignRoles: viewer.canAssignRoles,
    canAdminViewRoles: viewer.canAdminViewRoles,
    canAdminManageRoles: viewer.canAdminManageRoles,
    canAdminViewSystem: viewer.canAdminViewSystem,
    canAdminViewAudit: viewer.canAdminViewAudit,
    canManageUserPermissions: viewer.canManageUserPermissions,
    canManageInvites: viewer.canManageInvites,
    canManageServer: can('server.manage'),
    canManageNeighbors: can('server.manage-neighbors'),
    canManageRooms: can('room.manage'),
    canModerateRooms: can('room.remove-member'),
    canCreateBots: can('bot.create'),
    canManageBots: can('bot.manage')
  };
}
