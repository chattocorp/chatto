<!--
@component

General section of a role: its name, display name, description, ping
setting, and deletion. A role that the viewer cannot change shows read-only.
-->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { RoleDetails, UpdateRoleInput } from '@chatto/client/api/roles';
  import { DeleteRoleModal, type Role } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { completeMutation } from '$lib/navigation/mutationCompletion';
  import {
    invalidatePermissionTiers,
    removeDeletedRoleQueries
  } from '$lib/query/adminInvalidation';
  import { createMutation, queryClient } from '$lib/query/client';
  import { Hint } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { errorMessage } from '$lib/utils/errorMessage';
  import RoleMetadataPanel from './RoleMetadataPanel.svelte';
  import { useRoleDetail, type RoleMutationScope } from './roleDetailContext';

  type UpdateRoleVariables = RoleMutationScope & { input: UpdateRoleInput };

  const detail = useRoleDetail();
  const role = $derived(detail.role);
  let deleteConfirmRoleName = $state<string | null>(null);
  let metadataRevision = $state(0);

  function updateRoleSnapshot(variables: RoleMutationScope, updatedRole: Role): void {
    if (!detail.isCurrentRole(variables)) return;
    queryClient.setQueryData<RoleDetails>(variables.queryKey, (current) =>
      current ? { ...current, role: updatedRole } : current
    );
    invalidatePermissionTiers(variables.serverId, variables.connection);
  }

  const metadataMutation = createMutation(() => ({
    mutationFn: ({ api, input }: UpdateRoleVariables) => api.updateRole(input),
    onSuccess: (updatedRole, variables) => {
      updateRoleSnapshot(variables, updatedRole);
      if (detail.isCurrentRole(variables)) metadataRevision += 1;
    }
  }));

  const pingableMutation = createMutation(() => ({
    mutationFn: ({ api, input }: UpdateRoleVariables) => api.updateRole(input),
    onSuccess: (updatedRole, variables) => {
      updateRoleSnapshot(variables, updatedRole);
      if (detail.isCurrentRole(variables)) {
        toast.success(
          updatedRole.pingable
            ? m('admin.permissions.role_pings_on')
            : m('admin.permissions.role_pings_off')
        );
      }
    }
  }));

  const deleteMutation = createMutation(() => ({
    mutationFn: (variables: RoleMutationScope) =>
      completeMutation(
        () => variables.api.deleteRole(variables.roleName),
        variables.canComplete,
        () => {
          removeDeletedRoleQueries(variables.serverId, variables.connection, variables.roleName);
          void goto(
            resolve('/chat/[serverId]/manage/server/roles', {
              serverId: serverIdToSegment(variables.serverId)
            })
          );
        }
      ),
    onError: (_error, variables) => {
      if (detail.isCurrentRole(variables)) deleteConfirmRoleName = null;
    }
  }));

  const saving = $derived(
    metadataMutation.isPending && detail.isCurrentRole(metadataMutation.variables)
  );
  const savingPingable = $derived(
    pingableMutation.isPending && detail.isCurrentRole(pingableMutation.variables)
  );
  const deleting = $derived(
    deleteMutation.isPending && detail.isCurrentRole(deleteMutation.variables)
  );

  function saveMetadata(displayName: string, description: string): void {
    if (!detail.canEditRole || savingPingable) return;
    metadataMutation.mutate({
      ...detail.mutationScope(),
      input: { name: role.name, displayName, description }
    });
  }

  async function savePingable(nextPingable: boolean): Promise<boolean> {
    if (!detail.canEditRole || role.name === 'everyone' || saving) return false;
    if (nextPingable === role.pingable) return true;
    const variables = {
      ...detail.mutationScope(),
      input: {
        name: role.name,
        displayName: role.displayName,
        description: role.description,
        pingable: nextPingable
      }
    };
    try {
      await pingableMutation.mutateAsync(variables);
      return detail.isCurrentRole(variables);
    } catch {
      return false;
    }
  }

  function deleteRole() {
    if (!detail.canEditRole || role.isSystem) return;
    deleteMutation.mutate(detail.mutationScope());
  }

  const error = $derived.by(() => {
    if (metadataMutation.isError && detail.isCurrentRole(metadataMutation.variables)) {
      return errorMessage(metadataMutation.error, m('admin.permissions.update_role_failed'));
    }
    if (pingableMutation.isError && detail.isCurrentRole(pingableMutation.variables)) {
      return errorMessage(pingableMutation.error, m('admin.permissions.update_ping_failed'));
    }
    if (deleteMutation.isError && detail.isCurrentRole(deleteMutation.variables)) {
      return errorMessage(deleteMutation.error, m('admin.permissions.delete_role_failed'));
    }
    return null;
  });
</script>

{#if error}
  <FormError {error} />
{/if}

{#if role.name === 'everyone'}
  <Hint>{m('admin.permissions.everyone_implicit')}</Hint>
{/if}

<!-- The key resets the drafts for another role and after a successful save. -->
{#key `${role.name}:${metadataRevision}`}
  <RoleMetadataPanel
    {role}
    readOnly={!detail.canEditRole}
    {saving}
    {savingPingable}
    onSaveMetadata={saveMetadata}
    onSavePingable={savePingable}
    onDelete={() => (deleteConfirmRoleName = role.name)}
  />
{/key}

{#if deleteConfirmRoleName === role.name}
  <DeleteRoleModal
    roleDisplayName={role.displayName}
    {deleting}
    onConfirm={deleteRole}
    onCancel={() => (deleteConfirmRoleName = null)}
  />
{/if}
