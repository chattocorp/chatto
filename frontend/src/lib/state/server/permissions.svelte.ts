import { createContext } from 'svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from './registry.svelte';

/**
 * Viewer permissions data from the GraphQL `viewer` query.
 * This matches the shape returned by the Viewer type in the schema.
 */
export type ViewerData = {
  canViewAdmin: boolean;
  canViewDMs: boolean;
  canWriteDMs: boolean;
  canAdminViewUsers: boolean;
  canAdminManageUsers: boolean;
  canAdminViewRoles: boolean;
  canAdminManageRoles: boolean;
  canAdminViewSystem: boolean;
  canAdminViewAudit: boolean;
};

/**
 * Instance-level permissions for the current user, plus a `loaded` flag.
 * The underlying state lives on the per-instance `ServerStateStore`
 * (populated by `ServerSpaceSection`'s viewer query).
 */
export type ServerPermissions = ViewerData & {
  loaded: boolean;
};

const EMPTY_PERMISSIONS: ServerPermissions = {
  loaded: false,
  canViewAdmin: false,
  canViewDMs: false,
  canWriteDMs: false,
  canAdminViewUsers: false,
  canAdminManageUsers: false,
  canAdminViewRoles: false,
  canAdminManageRoles: false,
  canAdminViewSystem: false,
  canAdminViewAudit: false
};

/**
 * Returns a reactive view of the active instance's viewer permissions.
 *
 * Reads always resolve against the *active* instance (per the URL), so
 * navigating between instances reflects each instance's own permissions —
 * no origin-only context. Must be called during component initialization so
 * the underlying `getActiveServer()` context lookup succeeds.
 *
 * Usage:
 * ```ts
 * const instancePerms = getServerPermissions();
 * const canViewAdmin = $derived(instancePerms.current.canViewAdmin);
 * ```
 */
export function getServerPermissions(): { readonly current: ServerPermissions } {
  const getActiveId = getActiveServer();
  return {
    get current() {
      return serverRegistry.tryGetStore(getActiveId())?.permissions ?? EMPTY_PERMISSIONS;
    }
  };
}

/**
 * Maps a permission string constant to the corresponding typed boolean on ViewerData.
 * Used by the admin layout to bridge its string-based nav/route system.
 */
const PERMISSION_TO_FIELD: Record<string, keyof ViewerData> = {
  'admin.access': 'canViewAdmin',
  'dm.view': 'canViewDMs',
  'dm.write': 'canWriteDMs',
  'admin.view-users': 'canAdminViewUsers',
  'admin.manage-users': 'canAdminManageUsers',
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
