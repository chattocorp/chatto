import { createContext } from 'svelte';

export type ChromePermissions = {
  /**
   * True once the space's permissions have been loaded from the server. Use
   * this to gate "Access Denied" / loading-skeleton rendering — defaulting
   * to false would flash a denial during the brief window between layout
   * mount and the validateSpace query returning.
   */
  loaded: boolean;
  hasAnyAdminPermission: boolean;
  canManage: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
};

const [getChromePermissionsState, setChromePermissionsState] = createContext<{
  current: ChromePermissions;
}>();

/**
 * Creates and sets the space permissions context.
 * Must be called synchronously during component initialization.
 * Returns a function to update the permissions.
 */
export function createChromePermissions(): (permissions: Omit<ChromePermissions, 'loaded'>) => void {
  const state = $state<{ current: ChromePermissions }>({
    current: {
      loaded: false,
      hasAnyAdminPermission: false,
      canManage: false,
      canManageRooms: false,
      canManageRoles: false,
      canAssignRoles: false
    }
  });
  setChromePermissionsState(state);

  return (permissions: Omit<ChromePermissions, 'loaded'>) => {
    state.current = { ...permissions, loaded: true };
  };
}

/**
 * Gets the reactive space permissions state from context.
 * Returns the wrapper object so consumers can access `.current` reactively.
 */
export function getChromePermissions(): { current: ChromePermissions } {
  return getChromePermissionsState();
}
