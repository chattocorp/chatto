<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
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
  import { Button, Checkbox, TextInput, TextArea, FormError } from '$lib/ui/form';
  import { DeleteRoleModal, RolePermissionsMatrix, type Role } from '$lib/components/rbac';
  import {
    invalidatePermissionTiers,
    removeDeletedRoleQueries
  } from '$lib/query/adminInvalidation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import * as m from '$lib/i18n/messages';

  type User = RoleUser;

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const roleName = $derived(page.params.name!);

  type RoleMutationScope = {
    serverId: string;
    connection: ServerConnection;
    roleName: string;
    queryKey: ReturnType<typeof adminQueryKeys.role>;
    api: ReturnType<typeof createRoleAPI>;
  };

  type UpdateRoleVariables = RoleMutationScope & {
    input: UpdateRoleInput;
    previousPingable?: boolean;
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

  // Writable derived drafts reset automatically when a reused route receives a new snapshot.
  let editDisplayName = $derived(role?.displayName ?? '');
  let editDescription = $derived(role?.description ?? '');
  let editPingable = $derived(role?.pingable ?? false);

  function isCurrentSession(variables: RoleMutationScope | undefined): variables is RoleMutationScope {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection === serverScope.connection
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
      onSuccess: (updatedRole, variables) => updateRoleSnapshot(variables, updatedRole)
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
      },
      onError: (_error, variables) => {
        if (isCurrentRole(variables)) editPingable = variables.previousPingable ?? false;
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
      api: connection.getAPI(createRoleAPI)
    };
  }

  function saveMetadata() {
    if (!role || savingPingable) return;
    metadataMutation.mutate({
      ...mutationScope(role),
      input: {
        name: role.name,
        displayName: editDisplayName,
        description: editDescription
      }
    });
  }

  function savePingable(event: Event) {
    if (!role || !canEditPingable || saving) return;

    const target = event.currentTarget as HTMLInputElement;
    const nextPingable = target.checked;
    if (nextPingable === role.pingable) return;

    pingableMutation.mutate({
      ...mutationScope(role),
      previousPingable: role.pingable,
      input: {
        name: role.name,
        displayName: role.displayName,
        description: role.description,
        pingable: nextPingable
      }
    });
  }

  function deleteRole() {
    if (!role || role.isSystem) return;
    deleteMutation.mutate(mutationScope(role));
  }

  const permissionsHref = $derived(
    resolve('/chat/[serverId]/manage/server/permissions', { serverId: serverSegment })
  );

  const metadataChanged = $derived(
    role && (editDisplayName !== role.displayName || editDescription !== role.description)
  );
  const canEditPingable = $derived(role?.name !== 'everyone');
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
      <Panel title={m['admin.common.role_details']()} icon="iconify uil--info-circle">
        <div class="flex flex-col gap-4">
          <div>
            <div class="mb-1 text-sm font-medium">{m['rbac.role_form.name']()}</div>
            <code class="rounded bg-surface-emphasized px-2 py-1">{role.name}</code>
            <p class="mt-1 text-xs text-muted">{m['rbac.role_form.name_locked']()}</p>
          </div>

          {#if role.isSystem}
            <div>
              <div class="mb-1 text-sm font-medium">{m['rbac.role_form.display_name']()}</div>
              <div class="text-text">{role.displayName}</div>
            </div>
            <div>
              <div class="mb-1 text-sm font-medium">{m['rbac.role_form.description']()}</div>
              <div class="text-muted">{role.description}</div>
            </div>
            <p class="text-sm text-muted">{m['admin.permissions.system_metadata_locked']()}</p>
            <Checkbox
              id="pingable"
              bind:checked={editPingable}
              label={m['rbac.role_form.pingable']()}
              onchange={savePingable}
              disabled={saving || savingPingable || !canEditPingable}
              description={canEditPingable
                ? m['rbac.role_form.pingable_description']()
                : m['admin.permissions.everyone_pingable_description']()}
            />
          {:else}
            <TextInput
              id="displayName"
              testid="role-form-display-name"
              label={m['rbac.role_form.display_name']()}
              bind:value={editDisplayName}
            />
            <TextArea
              id="description"
              testid="role-form-description"
              label={m['rbac.role_form.description']()}
              bind:value={editDescription}
            />
            <Checkbox
              id="pingable"
              bind:checked={editPingable}
              label={m['rbac.role_form.pingable']()}
              onchange={savePingable}
              disabled={saving || savingPingable || !canEditPingable}
              description={canEditPingable
                ? m['rbac.role_form.pingable_description']()
                : m['admin.permissions.everyone_pingable_description']()}
            />
            <div class="flex gap-2">
              <Button
                variant="neutral"
                disabled={!metadataChanged || saving || savingPingable}
                onclick={saveMetadata}
              >
                {saving ? m['rbac.role_form.saving']() : m['admin.permissions.save_changes']()}
              </Button>
            </div>

            <!-- Delete Role -->
            <div class="mt-4 border-t border-border pt-4">
              <div class="mb-2 text-sm font-medium text-danger">
                {m['admin.common.danger_zone']()}
              </div>
              <p class="mb-3 text-sm text-muted">
                {m['admin.permissions.delete_role_description']()}
              </p>
              <Button variant="danger" onclick={() => (deleteConfirmRoleName = role.name)}>
                {m['rbac.delete_role.action']()}
              </Button>
            </div>
          {/if}
        </div>
      </Panel>

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
