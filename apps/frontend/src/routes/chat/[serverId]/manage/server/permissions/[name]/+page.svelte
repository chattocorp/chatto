<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    createRoleAPI,
    type RoleDetails,
    type RoleUser,
    type UpdateRoleInput
  } from '$lib/api-client/roles';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { Panel, UserList } from '$lib/components/admin';
  import { Hint, PaneContent } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { DeleteRoleModal, RolePermissionsMatrix, type Role } from '$lib/components/rbac';
  import {
    invalidatePermissionTiers,
    removeDeletedRoleQueries
  } from '$lib/query/adminInvalidation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import RoleMetadataPanel from './RoleMetadataPanel.svelte';
  import * as m from '$lib/i18n/messages';

  type User = RoleUser;

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const roleName = $derived(page.params.name!);
  let privacyGeneration = 0;
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    privacyGeneration += 1;
    removeCacheRemovalListener();
  });

  type RoleMutationScope = {
    serverId: string;
    connection: ServerConnection;
    roleName: string;
    queryKey: ReturnType<typeof adminQueryKeys.role>;
    api: ReturnType<typeof createRoleAPI>;
    privacyGeneration: number;
  };

  type UpdateRoleVariables = RoleMutationScope & {
    input: UpdateRoleInput;
  };

  const roleQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const targetRoleName = roleName;
      return {
        queryKey: adminQueryKeys.role(serverId, connection, targetRoleName),
        queryFn: ({ signal }) =>
          connection.getAPI(createRoleAPI).getRole(targetRoleName, { signal })
      };
    },
    () => queryClient
  );

  const roleDetails = $derived(roleQuery.data ?? null);
  const role = $derived((roleDetails?.role ?? null) as Role | null);
  const roleUsers = $derived((roleDetails?.users ?? []) as User[]);
  const canManageRoles = $derived(roleDetails?.viewerCanManageRoles ?? false);
  const canAssignRoles = $derived(roleDetails?.viewerCanAssignRoles ?? false);
  const loading = $derived(roleQuery.isPending);
  let deleteConfirmRoleName = $state<string | null>(null);
  let metadataRevision = $state(0);

  function isCurrentSession(variables: RoleMutationScope | undefined): variables is RoleMutationScope {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  function isCurrentRole(variables: RoleMutationScope | undefined): variables is RoleMutationScope {
    return isCurrentSession(variables) && variables.roleName === roleName;
  }

  function updateRoleSnapshot(variables: RoleMutationScope, updatedRole: Role): void {
    if (!isCurrentSession(variables)) return;
    queryClient.setQueryData<RoleDetails>(variables.queryKey, (current) =>
      current ? { ...current, role: updatedRole } : current
    );
    invalidatePermissionTiers(variables.serverId, variables.connection);
  }

  const metadataMutation = createMutation(
    () => ({
      mutationFn: ({ api, input }: UpdateRoleVariables) => api.updateRole(input),
      onSuccess: (updatedRole, variables) => {
        updateRoleSnapshot(variables, updatedRole);
        if (isCurrentRole(variables)) metadataRevision += 1;
      }
    }),
    () => queryClient
  );

  const pingableMutation = createMutation(
    () => ({
      mutationFn: ({ api, input }: UpdateRoleVariables) => api.updateRole(input),
      onSuccess: (updatedRole, variables) => {
        updateRoleSnapshot(variables, updatedRole);
        if (isCurrentRole(variables)) {
          toast.success(updatedRole.pingable ? 'Role pings enabled' : 'Role pings disabled');
        }
      }
    }),
    () => queryClient
  );

  const deleteMutation = createMutation(
    () => ({
      mutationFn: ({ api, roleName: targetRoleName }: RoleMutationScope) =>
        api.deleteRole(targetRoleName),
      onSuccess: (_deleted, variables) => {
        if (!isCurrentSession(variables)) return;
        removeDeletedRoleQueries(variables.serverId, variables.connection, variables.roleName);
        if (isCurrentRole(variables)) {
          goto(resolve('/chat/[serverId]/manage/server/permissions', { serverId: serverSegment }));
        }
      },
      onError: (_error, variables) => {
        if (isCurrentRole(variables)) deleteConfirmRoleName = null;
      }
    }),
    () => queryClient
  );

  function mutationScope(targetRole: Role): RoleMutationScope {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      serverId,
      connection,
      roleName: targetRole.name,
      queryKey: adminQueryKeys.role(serverId, connection, targetRole.name),
      api: connection.getAPI(createRoleAPI),
      privacyGeneration
    };
  }

  function saveMetadata(displayName: string, description: string): void {
    if (!role || savingPingable) return;
    metadataMutation.mutate({
      ...mutationScope(role),
      input: {
        name: role.name,
        displayName,
        description
      }
    });
  }

  async function savePingable(nextPingable: boolean): Promise<boolean> {
    if (!role || role.name === 'everyone' || saving) return false;
    if (nextPingable === role.pingable) return true;
    const variables = {
      ...mutationScope(role),
      input: {
        name: role.name,
        displayName: role.displayName,
        description: role.description,
        pingable: nextPingable
      }
    };
    try {
      await pingableMutation.mutateAsync(variables);
      return isCurrentRole(variables);
    } catch {
      return false;
    }
  }

  function deleteRole() {
    if (!role || role.isSystem) return;
    deleteMutation.mutate(mutationScope(role));
  }

  const permissionsHref = $derived(
    resolve('/chat/[serverId]/manage/server/permissions', { serverId: serverSegment })
  );

  const saving = $derived(
    metadataMutation.isPending && isCurrentRole(metadataMutation.variables)
  );
  const savingPingable = $derived(
    pingableMutation.isPending && isCurrentRole(pingableMutation.variables)
  );
  const deleting = $derived(deleteMutation.isPending && isCurrentRole(deleteMutation.variables));
  const error = $derived.by(() => {
    if (roleQuery.error) {
      return roleQuery.error instanceof Error ? roleQuery.error.message : String(roleQuery.error);
    }
    if (metadataMutation.isError && isCurrentRole(metadataMutation.variables)) {
      return metadataMutation.error instanceof Error
        ? metadataMutation.error.message
        : 'Failed to update role';
    }
    if (pingableMutation.isError && isCurrentRole(pingableMutation.variables)) {
      return pingableMutation.error instanceof Error
        ? pingableMutation.error.message
        : 'Failed to update role ping setting';
    }
    if (deleteMutation.isError && isCurrentRole(deleteMutation.variables)) {
      return deleteMutation.error instanceof Error
        ? deleteMutation.error.message
        : 'Failed to delete role';
    }
    return null;
  });
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({
    title: role?.displayName ?? m['admin.permissions.edit_role_title']()
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m['admin.permissions.edit_role_title']()}
    subtitle={role?.displayName ?? m['common.loading']()}
    backHref={permissionsHref}
    backLabel={m['admin.permissions.back_to_permissions']()}
    showMobileNav
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
    {#if loading}
      <div class="text-muted">{m['admin.permissions.loading_role']()}</div>
    {:else if !role}
      <div class="text-danger">{m['admin.permissions.role_not_found']()}</div>
    {:else if !canManageRoles}
      <div class="text-danger">
        {m['admin.permissions.need_manage_edit']()}
      </div>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <!-- Role Metadata -->
      {#key `${role.name}:${metadataRevision}`}
        <RoleMetadataPanel
          {role}
          {saving}
          {savingPingable}
          onSaveMetadata={saveMetadata}
          onSavePingable={savePingable}
          onDelete={() => (deleteConfirmRoleName = role.name)}
        />
      {/key}

      <!-- Permissions matrix: full per-role allow/deny across server, groups, and rooms. -->
      {#if canManageRoles && role}
        <Hint>
          {#if role.name === 'owner'}
            {m['admin.permissions.owner_permissions_hint']()}
          {:else}
            {m['admin.permissions.role_permissions_hint']()}
          {/if}
        </Hint>
        <RolePermissionsMatrix roleName={role.name} />
      {/if}

      <!-- Users with this role -->
      <Panel title={m['admin.permissions.users_with_role']()} icon="iconify uil--users-alt">
        {#if role?.name === 'everyone'}
          <p class="text-muted">{m['admin.permissions.everyone_implicit']()}</p>
        {:else}
          <UserList
            users={roleUsers}
            clickable={canAssignRoles}
            emptyMessage={m['admin.permissions.no_users_with_role']()}
            onUserClick={(user) =>
              goto(
                resolve('/chat/[serverId]/manage/server/members/[userId]', {
                  serverId: serverSegment,
                  userId: user.id
                })
              )}
          />
        {/if}
      </Panel>
    {/if}
    </div>
  </PaneContent>
</div>

<!-- Delete Confirmation Dialog -->
{#if deleteConfirmRoleName === role?.name && role}
  <DeleteRoleModal
    roleDisplayName={role.displayName}
    {deleting}
    onConfirm={deleteRole}
    onCancel={() => (deleteConfirmRoleName = null)}
  />
{/if}
