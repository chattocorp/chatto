import { createContext } from 'svelte';
import type { AdminMember, AdminMemberDetails, AdminUserManagementAPI } from '$lib/api/adminUsers';
import type { adminQueryKeys } from '$lib/query/admin';
import type { SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';

/** The session, member, and API that one member mutation targets. */
export type MemberMutationScope = SessionSnapshot & {
  userId: string;
  queryKey: ReturnType<typeof adminQueryKeys.member>;
  api: AdminUserManagementAPI;
};

/**
 * The member that the member detail layout loads, shared with its section
 * pages.
 *
 * The layout owns the member query and the session guard. The section pages
 * are rendered only while the member is loaded. SvelteKit reuses the layout
 * and the section pages when only the user ID changes, so every mutation must
 * capture a `mutationScope()` before it starts and check `isCurrentTarget`
 * before it applies its result.
 */
export interface MemberDetailContext {
  /** User ID from the current route. */
  readonly userId: string;
  /** The loaded member details. The layout renders sections only when present. */
  readonly details: AdminMemberDetails;
  readonly member: AdminMember;
  /** True when the viewer is looking at their own account. */
  readonly isSelf: boolean;
  readonly isBot: boolean;
  readonly canAdminManageAccounts: boolean;
  readonly canViewMemberEmails: boolean;
  /** True when this page may offer account deletion. */
  readonly canDeleteHere: boolean;
  /** The last member read error, for mutations that refetch the member. */
  readonly loadError: Error | null;
  /** Captures the current session and member, or `null` without a member. */
  mutationScope(): MemberMutationScope | null;
  /** True while the session and the route still target the mutation's member. */
  isCurrentTarget(target: MemberMutationScope | undefined): target is MemberMutationScope;
  /** Updates the cached member of the mutation's target. */
  updateCachedMember(
    target: MemberMutationScope,
    update: (current: AdminMember) => AdminMember
  ): void;
  /** Refetches the member lists after a change to the member. */
  invalidateMemberLists(target: MemberMutationScope): void;
  /** Refetches the member list of one role. */
  invalidateRole(target: MemberMutationScope, roleName: string): void;
}

const [getMemberDetail, setMemberDetail] = createContext<MemberDetailContext>();

/** Provides the member detail context to the section pages. */
export function provideMemberDetail(context: MemberDetailContext): void {
  setMemberDetail(context);
}

/** Returns the member detail context provided by the member detail layout. */
export function useMemberDetail(): MemberDetailContext {
  return getMemberDetail();
}
