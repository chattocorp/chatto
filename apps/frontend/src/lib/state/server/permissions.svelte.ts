import { createContext } from 'svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from './registry.svelte';

/**
 * Viewer permissions data returned by the viewer API.
 */
export type ViewerData = {
  /** Whether the viewer has at least one admin-capability entry point. */
  canViewAdmin: boolean;
  canStartDMs: boolean;
  canAdminViewUsers: boolean;
  canAdminManageUsers: boolean;
  canAdminViewRoles: boolean;
  canAdminManageRoles: boolean;
  canAdminViewSystem: boolean;
  canAdminViewAudit: boolean;
};

/**
 * Server-level permissions for the current user, plus a `loaded` flag.
 * The underlying state lives on the per-server `ServerStateStore`
 * (populated by `ServerSidebarEntry`'s viewer query).
 */
export type ServerPermissions = ViewerData & {
  loaded: boolean;
};

const EMPTY_PERMISSIONS: ServerPermissions = {
  loaded: false,
  canViewAdmin: false,
  canStartDMs: false,
  canAdminViewUsers: false,
  canAdminManageUsers: false,
  canAdminViewRoles: false,
  canAdminManageRoles: false,
  canAdminViewSystem: false,
  canAdminViewAudit: false
};

/**
 * Returns a reactive view of a server's viewer permissions.
 *
 * Defaults to the *active* server (per the URL `[serverId]` segment) so
 * navigating between servers reflects each one's own permissions. Pass an
 * explicit `serverId` to read a specific server's permissions — used by
 * unit tests, and by anywhere that needs to introspect a known server
 * regardless of the current URL.
 *
 * Usage:
 * ```ts
 * const serverPerms = getServerPermissions();
 * const canViewAdmin = $derived(serverPerms.current.canViewAdmin);
 * ```
 */
export function getServerPermissions(serverId?: string): { readonly current: ServerPermissions } {
  return {
    get current() {
      const id = serverId ?? getActiveServer();
      return serverRegistry.tryGetStore(id)?.permissions ?? EMPTY_PERMISSIONS;
    }
  };
}

/**
 * Maps a permission string constant to the corresponding typed boolean on ViewerData.
 * Used by the admin layout to bridge its string-based nav/route system.
 */
const PERMISSION_TO_FIELD: Record<string, keyof ViewerData> = {
  'admin.view-users': 'canAdminViewUsers',
  'role.assign': 'canAdminManageUsers',
  'role.manage': 'canAdminManageRoles',
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
