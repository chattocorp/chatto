<!--
@component

Roles section of a human member: the server role assignments.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import type { AdminRoleMutationResult } from '$lib/api-client/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createMutation, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import MemberRoleAssignments from '../../MemberRoleAssignments.svelte';
  import { useMemberDetail, type MemberMutationScope } from '../../memberDetailContext';

  const serverScope = useServerScope();
  const detail = useMemberDetail();
  let roleError = $state<{ targetKey: string; message: string } | null>(null);
  const visibleRoleError = $derived(
    roleError?.targetKey === detail.userId ? roleError.message : null
  );

  type RoleMutationVariables = MemberMutationScope & {
    roleName: string;
    currentlyHasRole: boolean;
  };

  const roleMutation = createMutation(() => ({
    mutationFn: ({
      api,
      userId: targetUserId,
      roleName,
      currentlyHasRole
    }: RoleMutationVariables) =>
      currentlyHasRole
        ? api.revokeRole(targetUserId, roleName)
        : api.assignRole(targetUserId, roleName)
  }));

  async function toggleMemberRole(roleName: string, currentlyHasRole: boolean): Promise<boolean> {
    const target = detail.mutationScope();
    if (!target || (roleMutation.isPending && detail.isCurrentTarget(roleMutation.variables)))
      return false;
    const targetKey = detail.userId;
    roleError = null;

    let result: AdminRoleMutationResult;
    try {
      result = await roleMutation.mutateAsync({ ...target, roleName, currentlyHasRole });
    } catch (error) {
      if (detail.isCurrentTarget(target)) {
        roleError = {
          targetKey,
          message: errorMessage(error, m('admin.members.role_update_failed'))
        };
      }
      return false;
    }
    if (!detail.isCurrentTarget(target) || !result.changed) return false;

    if (result.member) {
      detail.updateCachedMember(target, () => result.member!);
    } else {
      await queryClient.invalidateQueries({ queryKey: target.queryKey, exact: true });
      if (detail.isCurrentTarget(target) && detail.loadError) {
        roleError = {
          targetKey,
          message: errorMessage(detail.loadError, m('admin.members.load_failed'))
        };
      }
    }

    detail.invalidateMemberLists(target);
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.userPermissions(target.serverId, target.connection, target.userId),
      exact: true
    });
    detail.invalidateRole(target, roleName);
    return detail.isCurrentTarget(target);
  }

  const updatingRole = $derived(
    roleMutation.isPending && detail.isCurrentTarget(roleMutation.variables)
      ? (roleMutation.variables?.roleName ?? null)
      : null
  );
</script>

{#if detail.isBot}
  <Hint tone="danger">{m('ui.access_denied.message')}</Hint>
{:else}
  {#if visibleRoleError}
    <FormError error={visibleRoleError} />
  {/if}
  {#key detail.userId}
    <MemberRoleAssignments
      details={detail.details}
      isSelf={detail.isSelf}
      serverId={serverScope.serverId}
      {updatingRole}
      {toggleMemberRole}
    />
  {/key}
{/if}
