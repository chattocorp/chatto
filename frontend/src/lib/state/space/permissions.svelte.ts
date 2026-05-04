import { createContext } from 'svelte';

export type SpacePermissions = {
  /**
   * True once the space's permissions have been loaded from the server. Use
   * this to gate "Access Denied" / loading-skeleton rendering — defaulting
   * to false would flash a denial during the brief window between layout
   * mount and the validateSpace query returning.
   */
  loaded: boolean;
  hasAnyAdminPermission: boolean;
  canManage: boolean;
  canBrowseRooms: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
  canInviteMembers: boolean;
};

const [getSpacePermissionsState, setSpacePermissionsState] = createContext<{
  current: SpacePermissions;
}>();

/**
 * Creates and sets the space permissions context.
 * Must be called synchronously during component initialization.
 * Returns a function to update the permissions.
 */
export function createSpacePermissions(): (permissions: Omit<SpacePermissions, 'loaded'>) => void {
  const state = $state<{ current: SpacePermissions }>({
    current: {
      loaded: false,
      hasAnyAdminPermission: false,
      canManage: false,
      canBrowseRooms: false,
      canManageRooms: false,
      canManageRoles: false,
      canAssignRoles: false,
      canInviteMembers: false
    }
  });
  setSpacePermissionsState(state);

  return (permissions: Omit<SpacePermissions, 'loaded'>) => {
    state.current = { ...permissions, loaded: true };
  };
}

/**
 * Gets the reactive space permissions state from context.
 * Returns the wrapper object so consumers can access `.current` reactively.
 */
export function getSpacePermissions(): { current: SpacePermissions } {
  return getSpacePermissionsState();
}
