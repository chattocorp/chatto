import { createContext } from 'svelte';

export type ChromePermissions = {
  /**
   * True once the server chrome permissions have been loaded. Use
   * this to gate "Access Denied" / loading-skeleton rendering — defaulting
   * to false would flash a denial during the brief window between layout
   * mount and the validateServer query returning.
   */
  loaded: boolean;
  canViewAdmin: boolean;
  canManage: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
  canManageUserPermissions: boolean;
};

const [getChromePermissionsState, setChromePermissionsState] = createContext<{
  current: ChromePermissions;
}>();

/**
 * Creates and sets the server chrome permissions context.
 * Must be called synchronously during component initialization.
 * Returns a function to update the permissions.
 */
export function createChromePermissions(): (permissions: Omit<ChromePermissions, 'loaded'>) => void {
  const state = $state<{ current: ChromePermissions }>({
    current: {
      loaded: false,
      canViewAdmin: false,
      canManage: false,
      canManageRooms: false,
      canManageRoles: false,
      canAssignRoles: false,
      canManageUserPermissions: false
    }
  });
  setChromePermissionsState(state);

  return (permissions: Omit<ChromePermissions, 'loaded'>) => {
    state.current = { ...permissions, loaded: true };
  };
}

/**
 * Gets the reactive server chrome permissions state from context.
 * Returns the wrapper object so consumers can access `.current` reactively.
 */
export function getChromePermissions(): { current: ChromePermissions } {
  return getChromePermissionsState();
}
