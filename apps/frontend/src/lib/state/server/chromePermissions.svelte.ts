import { createContext } from 'svelte';

export type ChromePermissions = {
  canViewAdmin: boolean;
  canManage: boolean;
  canManageNeighbors: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
  canManageUserAccounts: boolean;
  canManageUserPermissions: boolean;
};

export type ChromePermissionsState = ChromePermissions | null;

const [getChromePermissionsGetter, setChromePermissionsGetter] =
  createContext<() => ChromePermissionsState>();

/**
 * Creates and sets the server chrome permissions context.
 * Must be called synchronously during component initialization.
 *
 * The getter returns `null` until permissions are available, allowing consumers
 * to distinguish loading from an explicit denial.
 */
export function createChromePermissions(getPermissions: () => ChromePermissionsState): void {
  setChromePermissionsGetter(getPermissions);
}

/** Gets the reactive server chrome permissions getter from context. */
export function getChromePermissions(): () => ChromePermissionsState {
  return getChromePermissionsGetter();
}
