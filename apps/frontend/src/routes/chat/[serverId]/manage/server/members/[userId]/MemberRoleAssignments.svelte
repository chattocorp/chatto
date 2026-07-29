<script lang="ts">
  import { resolve } from '$app/paths';
  import { Panel } from '$lib/components/admin';
  import * as m from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { Checkbox } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import type { MemberDetailStore } from './MemberDetailStore.svelte';

  type Props = {
    store: MemberDetailStore;
    isSelf: boolean;
    serverId: string;
  };

  let { store, isSelf, serverId }: Props = $props();

  const memberRoles = $derived(store.member?.roles ?? []);

  function hasRole(roleName: string): boolean {
    return memberRoles.includes(roleName);
  }

  function isImplicitRole(roleName: string): boolean {
    return roleName === 'everyone';
  }

  async function toggleRole(roleName: string, currentlyHasRole: boolean): Promise<void> {
    if (!(await store.toggleRole(roleName, currentlyHasRole))) return;

    const displayName = store.roles.find((role) => role.name === roleName)?.displayName ?? roleName;
    toast.success(
      currentlyHasRole
        ? m['admin.members.removed_role']({ role: displayName })
        : m['admin.members.assigned_role']({ role: displayName })
    );
  }
</script>

<Panel title={m['admin.members.role_assignments']()} icon="iconify uil--shield-check">
  <p class="mb-4 text-sm text-muted">
    {store.canAssignRoles
      ? m['admin.members.assign_roles_description']()
      : m['admin.members.view_roles_description']()}
  </p>

  <div class="flex flex-col gap-2">
    {#each store.roles as role (role.name)}
      {@const isImplicit = isImplicitRole(role.name)}
      {@const has = isImplicit || hasRole(role.name)}
      {@const isUpdating = store.updatingRole === role.name}
      {@const isSelfProtectedRole =
        isSelf && (role.name === 'admin' || role.name === 'owner') && has}
      {@const isWithinAssignmentAuthority = has
        ? store.revocableRoleNames === null || store.revocableRoleNames.includes(role.name)
        : store.assignableRoleNames === null || store.assignableRoleNames.includes(role.name)}
      {@const isDisabled =
        !store.canAssignRoles ||
        isImplicit ||
        isUpdating ||
        isSelfProtectedRole ||
        !isWithinAssignmentAuthority}
      {@const tooltip = isImplicit
        ? m['admin.members.implicit_role_tooltip']()
        : isSelfProtectedRole
          ? m['admin.members.cannot_revoke_own_role']({ role: role.displayName })
          : !isWithinAssignmentAuthority
            ? m['ui.access_denied.message']()
            : ''}

      <div class="flex items-center gap-3">
        <div class="min-w-0 flex-1" title={tooltip}>
          <Checkbox
            id={`role-assignment-${role.name}`}
            checked={has}
            disabled={isDisabled}
            loading={isUpdating}
            onchange={() => toggleRole(role.name, has)}
          >
            <span class="block">{role.displayName}</span>
            {#if isImplicit}
              <span class="block text-xs font-normal text-muted">
                {m['admin.members.implicit_all_members']()}
              </span>
            {/if}
          </Checkbox>
        </div>
        {#if store.canManageRoles}
          <a
            href={resolve('/chat/[serverId]/manage/server/permissions/[name]', {
              serverId: serverIdToSegment(serverId),
              name: role.name
            })}
            class="shrink-0 text-sm link"
          >
            {m['admin.members.edit']()}
          </a>
        {/if}
      </div>
    {/each}
  </div>
</Panel>
