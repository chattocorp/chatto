import { createContext } from 'svelte';

/**
 * Viewer permissions data from the GraphQL `viewer` query.
 * This matches the shape returned by the Viewer type in the schema.
 */
export type ViewerData = {
  canViewAdmin: boolean;
  canListSpaces: boolean;
  canViewDMs: boolean;
  canWriteDMs: boolean;
  canAdminViewUsers: boolean;
  canAdminManageUsers: boolean;
  canAdminViewSpaces: boolean;
  canAdminViewRoles: boolean;
  canAdminManageRoles: boolean;
  canAdminViewSystem: boolean;
  canAdminViewAudit: boolean;
};

/**
 * Instance-level permissions for the current user.
 * Set by the chat layout, consumed by child routes.
 *
 * Uses a reactive state object so the context can be set synchronously
 * during component initialization, then updated when the query completes.
 */
export type InstancePermissions = ViewerData & {
  loaded: boolean;
};

const [getPermissionsState, setPermissionsState] = createContext<{
  current: InstancePermissions;
}>();

const EMPTY_PERMISSIONS: InstancePermissions = {
  loaded: false,
  canViewAdmin: false,
  canListSpaces: false,
  canViewDMs: false,
  canWriteDMs: false,
  canAdminViewUsers: false,
  canAdminManageUsers: false,
  canAdminViewSpaces: false,
  canAdminViewRoles: false,
  canAdminManageRoles: false,
  canAdminViewSystem: false,
  canAdminViewAudit: false
};

/**
 * Creates and sets the instance permissions context.
 * Must be called synchronously during component initialization (chat layout).
 * Returns a function to update the permissions when the viewer query completes.
 */
export function createInstancePermissions(): (viewer: ViewerData) => void {
  const state = $state<{ current: InstancePermissions }>({
    current: EMPTY_PERMISSIONS
  });
  setPermissionsState(state);

  return (viewer: ViewerData) => {
    state.current = {
      ...viewer,
      loaded: true
    };
  };
}

/**
 * Gets the reactive instance permissions state from context.
 * Returns the wrapper object so consumers can access `.current` reactively.
 *
 * Usage in components:
 * ```ts
 * const instancePerms = getInstancePermissions();
 * const canViewAdmin = $derived(instancePerms.current.canViewAdmin);
 * ```
 */
export function getInstancePermissions(): { current: InstancePermissions } {
  return getPermissionsState();
}

/**
 * Maps a permission string constant to the corresponding typed boolean on ViewerData.
 * Used by the admin layout to bridge its string-based nav/route system.
 */
const PERMISSION_TO_FIELD: Record<string, keyof ViewerData> = {
  'admin.access': 'canViewAdmin',
  'space.list': 'canListSpaces',
  'dm.view': 'canViewDMs',
  'dm.write': 'canWriteDMs',
  'admin.view-users': 'canAdminViewUsers',
  'admin.manage-users': 'canAdminManageUsers',
  'admin.view-spaces': 'canAdminViewSpaces',
  'admin.view-roles': 'canAdminViewRoles',
  'admin.manage-roles': 'canAdminManageRoles',
  'admin.view-system': 'canAdminViewSystem',
  'admin.view-audit': 'canAdminViewAudit'
};

export function viewerHasPermission(viewer: ViewerData, perm: string): boolean {
  const key = PERMISSION_TO_FIELD[perm];
  return key ? viewer[key] : false;
}

// ---------------------------------------------------------------------------
// Admin Permissions — set by admin layout
// ---------------------------------------------------------------------------

export interface AdminPermissions {
  hasPermission(perm: string): boolean;
}

export const [getAdminPermissions, createAdminPermissions] = createContext<AdminPermissions>();
