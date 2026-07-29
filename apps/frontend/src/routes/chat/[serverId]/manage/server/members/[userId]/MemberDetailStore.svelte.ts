import type {
  AdminManagedUser,
  AdminMember,
  AdminMemberDetails,
  AdminRoleDetails,
  AdminUserManagementAPI
} from '$lib/api-client/adminUsers';
import * as m from '$lib/i18n/messages';

type APIProvider = () => AdminUserManagementAPI;
type MemberTarget = {
  serverId: string;
  userId: string;
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
  #serverId = '';
  #userId = '';
  #generation = 0;

  constructor(getAPI: APIProvider) {
    this.#getAPI = getAPI;
  }

  setMember(serverId: string, userId: string): Promise<void> {
    if (serverId === this.#serverId && userId === this.#userId) return Promise.resolve();

    this.#serverId = serverId;
    this.#userId = userId;
    const generation = ++this.#generation;
    this.#clear();

    if (!serverId || !userId) return Promise.resolve();

    this.loading = true;
    return this.#load({ serverId, userId, generation });
  }

  async updateIdentity(input: {
    login?: string;
    displayName?: string;
  }): Promise<AdminManagedUser | null> {
    const target = this.#target();
    if (!target || !this.member) return null;

    const updated = await this.#getAPI().updateUser({
      userId: target.userId,
      ...input
    });
    if (!this.#isCurrent(target) || !this.member) return null;

    this.member = {
      ...this.member,
      login: updated.login,
      displayName: updated.displayName
    };
    return updated;
  }

  async clearUsernameCooldown(): Promise<boolean> {
    const target = this.#target();
    if (!target || !this.member) return false;

    const cleared = await this.#getAPI().clearUsernameCooldown(target.userId);
    if (!cleared || !this.#isCurrent(target) || !this.member) return false;

    this.member = { ...this.member, lastLoginChange: null };
    return true;
  }

  async updatePassword(password: string): Promise<AdminMember | null> {
    const target = this.#target();
    if (!target || !this.member) return null;

    const updated = await this.#getAPI().updateUserPassword(target.userId, password);
    if (!this.#isCurrent(target)) return null;

    this.member = updated;
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
          ? await this.#getAPI().revokeRole(target.userId, roleName)
          : await this.#getAPI().assignRole(target.userId, roleName);
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
      } else {
        try {
          await this.#refresh(target);
        } catch (error) {
          if (this.#isCurrent(target)) {
            this.error = error instanceof Error ? error.message : m['admin.members.load_failed']();
          }
        }
      }

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
      const details = await this.#getAPI().getMember(target.userId);
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
    const details = await this.#getAPI().getMember(target.userId);
    if (this.#isCurrent(target)) this.#apply(details);
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
    return this.#serverId && this.#userId
      ? {
          serverId: this.#serverId,
          userId: this.#userId,
          generation: this.#generation
        }
      : null;
  }

  #isCurrent(target: MemberTarget): boolean {
    return (
      target.serverId === this.#serverId &&
      target.userId === this.#userId &&
      target.generation === this.#generation
    );
  }
}
