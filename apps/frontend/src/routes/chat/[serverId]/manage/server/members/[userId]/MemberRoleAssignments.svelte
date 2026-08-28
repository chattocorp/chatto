<script lang="ts">
  import { resolve } from '$app/paths';
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { Checkbox } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import type { AdminMemberDetails } from '$lib/api-client/adminUsers';

  type Props = {
    details: AdminMemberDetails;
    isSelf: boolean;
    serverId: string;
    updatingRole: string | null;
    toggleMemberRole: (roleName: string, currentlyHasRole: boolean) => Promise<boolean>;
  };

  let { details, isSelf, serverId, updatingRole, toggleMemberRole }: Props = $props();

  const memberRoles = $derived(details.member?.roles ?? []);

  function hasRole(roleName: string): boolean {
    return memberRoles.includes(roleName);
  }

  function isImplicitRole(roleName: string): boolean {
    return roleName === 'everyone';
  }

  async function toggleRole(roleName: string, currentlyHasRole: boolean): Promise<void> {
    if (!(await toggleMemberRole(roleName, currentlyHasRole))) return;

    const displayName =
      details.roles.find((role) => role.name === roleName)?.displayName ?? roleName;
    toast.success(
      currentlyHasRole
        ? m('admin.members.removed_role', { role: displayName })
        : m('admin.members.assigned_role', { role: displayName })
    );
  }
</script>

<Panel title={m('admin.members.role_assignments')} icon="iconify icon-[uil--shield-check]">
  <p class="mb-4 text-sm text-muted">
    {details.viewerCanAssignRoles
      ? m('admin.members.assign_roles_description')
      : m('admin.members.view_roles_description')}
  </p>

  <div class="flex flex-col gap-2">
    {#each details.roles as role (role.name)}
      {@const isImplicit = isImplicitRole(role.name)}
      {@const has = isImplicit || hasRole(role.name)}
      {@const isUpdating = updatingRole === role.name}
      {@const isSelfProtectedRole =
        isSelf && (role.name === 'admin' || role.name === 'owner') && has}
      {@const isWithinAssignmentAuthority = has
        ? details.revocableRoleNames === null || details.revocableRoleNames.includes(role.name)
        : details.assignableRoleNames === null || details.assignableRoleNames.includes(role.name)}
      {@const isDisabled =
        !details.viewerCanAssignRoles ||
        isImplicit ||
        isUpdating ||
        isSelfProtectedRole ||
        !isWithinAssignmentAuthority}
      {@const tooltip = isImplicit
        ? m('admin.members.implicit_role_tooltip')
        : isSelfProtectedRole
          ? m('admin.members.cannot_revoke_own_role', { role: role.displayName })
          : !isWithinAssignmentAuthority
            ? m('ui.access_denied.message')
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
                {m('admin.members.implicit_all_members')}
              </span>
            {/if}
          </Checkbox>
        </div>
        {#if details.viewerCanManageRoles}
          <a
            href={resolve('/chat/[serverId]/manage/server/permissions/[name]', {
              serverId: serverIdToSegment(serverId),
              name: role.name
            })}
            class="shrink-0 text-sm link"
          >
            {m('admin.members.edit')}
          </a>
        {/if}
      </div>
    {/each}
  </div>
</Panel>
