import { createContext } from 'svelte';
import type { createRoleAPI } from '@chatto/client/api/roles';
import type { Role } from '$lib/components/rbac';
import type { adminQueryKeys } from '$lib/query/admin';
import type { SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';

/** The session, role, and API that one role mutation targets. */
export type RoleMutationScope = SessionSnapshot & {
  roleName: string;
  queryKey: ReturnType<typeof adminQueryKeys.role>;
  api: ReturnType<typeof createRoleAPI>;
  /** False when the route or session changed, so the mutation must not navigate. */
  canComplete: () => boolean;
};

/**
 * The role that the role detail layout loads, shared with its section pages.
 *
 * The layout owns the role query and the session guard. The section pages are
 * rendered only while the role is loaded and the viewer can manage roles.
 * SvelteKit reuses the layout and the section pages when only the role name
 * changes, so every mutation must capture a `mutationScope()` before it starts
 * and check `isCurrentRole` before it applies its result.
 */
export interface RoleDetailContext {
  /** Role name from the current route. */
  readonly roleName: string;
  /** The loaded role. The layout renders sections only when present. */
  readonly role: Role;
  /** True when the viewer may assign this role, so they may see its members. */
  readonly canAssignRoles: boolean;
  /**
   * True when the viewer may change the role: they manage roles and the role
   * ranks below their highest role. Otherwise the sections are read-only.
   */
  readonly canEditRole: boolean;
  /** The scroll container of the page, for lists that load more at its edge. */
  readonly scrollContainer: HTMLDivElement | undefined;
  /** Captures the current session and role. */
  mutationScope(): RoleMutationScope;
  /** True while the session and the route still target the mutation's role. */
  isCurrentRole(target: RoleMutationScope | undefined): target is RoleMutationScope;
}

const [getRoleDetail, setRoleDetail] = createContext<RoleDetailContext>();

/** Provides the role detail context to the section pages. */
export function provideRoleDetail(context: RoleDetailContext): void {
  setRoleDetail(context);
}

/** Returns the role detail context provided by the role detail layout. */
export function useRoleDetail(): RoleDetailContext {
  return getRoleDetail();
}
