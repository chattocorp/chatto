import type {
  AdminManagedUser,
  AdminMember,
  AdminMemberDetails,
  AdminRoleDetails,
  AdminUserManagementAPI
} from '$lib/api-client/adminUsers';
import * as m from '$lib/i18n/messages';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';
import { registerAdminUserRemovalListener } from '$lib/query/cacheRegistry';

type APIProvider = () => AdminUserManagementAPI;
type QueryConnectionProvider = () => Pick<ServerConnection, 'queryScope'>;
type MemberTarget = {
  serverId: string;
  userId: string;
  queryScope: string;
  api: AdminUserManagementAPI;
  generation: number;
};

/**
 * Owns the member-detail page's read and mutation lifecycle.
 *
 * SvelteKit preserves the page while only the member route parameter changes,
 * so every async operation is fenced to the member and generation that started
 * it. Older responses must never restore the previous member's details.
 */
export class MemberDetailStore {
  member = $state.raw<AdminMember | null>(null);
  roles = $state.raw<AdminRoleDetails[]>([]);
  canAssignRoles = $state(false);
  canManageRoles = $state(false);
  canManageUserPermissions = $state(false);
  assignableRoleNames = $state.raw<string[] | null>(null);
  revocableRoleNames = $state.raw<string[] | null>(null);
  loading = $state(true);
  updatingRole = $state<string | null>(null);
  error = $state<string | null>(null);

  readonly #getAPI: APIProvider;
  readonly #getQueryConnection: QueryConnectionProvider;
  #serverId = '';
  #userId = '';
  #queryScope = '';
  #api: AdminUserManagementAPI | null = null;
  #generation = 0;
  readonly #removeUserRemovalListener: () => void;

  constructor(
    getAPI: APIProvider,
    getQueryConnection: QueryConnectionProvider = () => ({ queryScope: 'member-detail' })
  ) {
    this.#getAPI = getAPI;
    this.#getQueryConnection = getQueryConnection;
    this.#removeUserRemovalListener = registerAdminUserRemovalListener((serverId, userId) => {
      if (serverId !== this.#serverId || userId !== this.#userId) return;
      this.#generation += 1;
      this.#clear();
    });
  }

  dispose(): void {
    this.#generation += 1;
    this.#removeUserRemovalListener();
    this.#clear();
  }

  setMember(serverId: string, userId: string): Promise<void> {
    const queryScope = this.#getQueryConnection().queryScope;
    if (serverId === this.#serverId && userId === this.#userId && queryScope === this.#queryScope) {
      return Promise.resolve();
    }

    this.#serverId = serverId;
    this.#userId = userId;
    this.#queryScope = queryScope;
    const api = this.#getAPI();
    this.#api = api;
    const generation = ++this.#generation;
    this.#clear();

    if (!serverId || !userId) return Promise.resolve();

    this.loading = true;
    return this.#load({ serverId, userId, queryScope, api, generation });
  }

  async updateIdentity(input: {
    login?: string;
    displayName?: string;
  }): Promise<AdminManagedUser | null> {
    const target = this.#target();
    if (!target || !this.member) return null;

    const updated = await target.api.updateUser({
      userId: target.userId,
      ...input
    });
    if (!this.#isCurrent(target) || !this.member) return null;

    this.member = {
      ...this.member,
      login: updated.login,
      displayName: updated.displayName
    };
    this.#updateCachedMember(target, (member) => ({
      ...member,
      login: updated.login,
      displayName: updated.displayName
    }));
    this.#invalidateMemberLists(target);
    this.#invalidateRoleDetailsForMember(target, this.member.roles);
    return updated;
  }

  async clearUsernameCooldown(): Promise<boolean> {
    const target = this.#target();
    if (!target || !this.member) return false;

    const cleared = await target.api.clearUsernameCooldown(target.userId);
    if (!cleared || !this.#isCurrent(target) || !this.member) return false;

    this.member = { ...this.member, lastLoginChange: null };
    this.#updateCachedMember(target, (member) => ({ ...member, lastLoginChange: null }));
    this.#invalidateMemberLists(target);
    return true;
  }

  async updatePassword(password: string): Promise<AdminMember | null> {
    const target = this.#target();
    if (!target || !this.member) return null;

    const updated = await target.api.updateUserPassword(target.userId, password);
    if (!this.#isCurrent(target)) return null;

    this.member = updated;
    this.#updateCachedMember(target, () => updated);
    this.#invalidateMemberLists(target);
    return updated;
  }

  async toggleRole(roleName: string, currentlyHasRole: boolean): Promise<boolean> {
    const target = this.#target();
    if (!target || !this.member || this.updatingRole) return false;

    this.updatingRole = roleName;
    this.error = null;

    try {
      let result;
      try {
        result = currentlyHasRole
          ? await target.api.revokeRole(target.userId, roleName)
          : await target.api.assignRole(target.userId, roleName);
      } catch (error) {
        if (this.#isCurrent(target)) {
          this.error =
            error instanceof Error ? error.message : m['admin.members.role_update_failed']();
        }
        return false;
      }

      if (!this.#isCurrent(target) || !result.changed) return false;

      if (result.member) {
        this.member = result.member;
        this.#updateCachedMember(target, () => result.member!);
      } else {
        try {
          await this.#refresh(target);
        } catch (error) {
          if (this.#isCurrent(target)) {
            this.error = error instanceof Error ? error.message : m['admin.members.load_failed']();
          }
        }
      }

      this.#invalidateMemberLists(target);
      this.#invalidateUserPermissions(target);
      this.#invalidateRoleDetails(target, roleName);

      return this.#isCurrent(target);
    } finally {
      if (this.#isCurrent(target)) this.updatingRole = null;
    }
  }

  #clear(): void {
    this.member = null;
    this.roles = [];
    this.canAssignRoles = false;
    this.canManageRoles = false;
    this.canManageUserPermissions = false;
    this.assignableRoleNames = null;
    this.revocableRoleNames = null;
    this.loading = false;
    this.updatingRole = null;
    this.error = null;
  }

  async #load(target: MemberTarget): Promise<void> {
    try {
      const details = await queryClient.fetchQuery({
        queryKey: this.#queryKey(target),
        queryFn: ({ signal }) => target.api.getMember(target.userId, { signal })
      });
      if (!this.#isCurrent(target)) return;
      this.#apply(details);
    } catch (error) {
      if (!this.#isCurrent(target)) return;
      this.error = error instanceof Error ? error.message : m['admin.members.load_failed']();
    } finally {
      if (this.#isCurrent(target)) this.loading = false;
    }
  }

  async #refresh(target: MemberTarget): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: this.#queryKey(target), exact: true });
    const details = await queryClient.fetchQuery({
      queryKey: this.#queryKey(target),
      queryFn: ({ signal }) => target.api.getMember(target.userId, { signal }),
      retry: false
    });
    if (this.#isCurrent(target)) this.#apply(details);
  }

  #queryKey(target: Pick<MemberTarget, 'serverId' | 'userId' | 'queryScope'>) {
    return adminQueryKeys.member(target.serverId, target, target.userId);
  }

  #updateCachedMember(target: MemberTarget, update: (member: AdminMember) => AdminMember): void {
    queryClient.setQueryData<AdminMemberDetails>(this.#queryKey(target), (details) => {
      if (!details?.member) return details;
      return { ...details, member: update(details.member) };
    });
  }

  #invalidateMemberLists(target: MemberTarget): void {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.membersRoot(target.serverId, target)
    });
  }

  #invalidateUserPermissions(target: MemberTarget): void {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.userPermissions(target.serverId, target, target.userId),
      exact: true
    });
  }

  #invalidateRoleDetails(target: MemberTarget, roleName: string): void {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.role(target.serverId, target, roleName),
      exact: true
    });
  }

  #invalidateRoleDetailsForMember(target: MemberTarget, roleNames: string[]): void {
    for (const roleName of roleNames) this.#invalidateRoleDetails(target, roleName);
  }

  #apply(details: AdminMemberDetails): void {
    this.member = details.member;
    this.roles = details.roles;
    this.canAssignRoles = details.viewerCanAssignRoles;
    this.canManageRoles = details.viewerCanManageRoles;
    this.canManageUserPermissions = details.viewerCanManageUserPermissions;
    this.assignableRoleNames = details.assignableRoleNames;
    this.revocableRoleNames = details.revocableRoleNames;
  }

  #target(): MemberTarget | null {
    return this.#serverId && this.#userId && this.#api
      ? {
          serverId: this.#serverId,
          userId: this.#userId,
          queryScope: this.#queryScope,
          api: this.#api,
          generation: this.#generation
        }
      : null;
  }

  #isCurrent(target: MemberTarget): boolean {
    return (
      target.serverId === this.#serverId &&
      target.userId === this.#userId &&
      target.queryScope === this.#queryScope &&
      target.generation === this.#generation
    );
  }
}
