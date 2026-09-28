<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { goto } from '$app/navigation';
  import { captureMutationCompletion, completeMutation } from '$lib/navigation/mutationCompletion';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createInfiniteQuery, createMutation, createQuery } from '@tanstack/svelte-query';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createRoleAPI, type RoleDetails, type UpdateRoleInput } from '$lib/api-client/roles';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { UserList } from '$lib/components/admin';
  import Panel from '$lib/ui/Panel.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
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
  import RoleMetadataPanel from './RoleMetadataPanel.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const roleName = $derived(page.params.name!);
  const session = createSessionGuard(serverScope);

  type RoleMutationScope = SessionSnapshot & {
    roleName: string;
    queryKey: ReturnType<typeof adminQueryKeys.role>;
    api: ReturnType<typeof createRoleAPI>;
    canComplete: () => boolean;
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

  const canManageRoles = $derived(roleDetails?.viewerCanManageRoles ?? false);
  const canAssignRoles = $derived(roleDetails?.viewerCanAssignRoles ?? false);
  let scrollContainer = $state<HTMLDivElement>();
  const membersQuery = createInfiniteQuery(
    () => {
      const connection = serverScope.connection;
      const name = roleName;
      return {
        queryKey: adminQueryKeys.roleMembers(serverScope.serverId, connection, name),
        enabled: canAssignRoles && name !== 'everyone',
        queryFn: ({ pageParam, signal }) =>
          connection
            .getAPI(createRoleAPI)
            .listMembers(name, { limit: 20, offset: pageParam }, { signal }),
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, offset) =>
          lastPage.hasMore && lastPage.users.length > 0 ? offset + lastPage.users.length : undefined
      };
    },
    () => queryClient
  );
  const roleUsers = $derived.by(() => {
    const users = (membersQuery.data?.pages ?? []).flatMap((page) => page.users);
    return [...new Map(users.map((user) => [user.id, user])).values()];
  });
  const membersError = $derived(membersQuery.error ? errorMessage(membersQuery.error) : null);
  async function loadMoreMembers() {
    if (membersQuery.hasNextPage && !membersQuery.isFetching && !membersError) {
      await membersQuery.fetchNextPage();
    }
  }
  const loading = $derived(roleQuery.isPending);
  let deleteConfirmRoleName = $state<string | null>(null);
  let metadataRevision = $state(0);

  function isCurrentRole(variables: RoleMutationScope | undefined): variables is RoleMutationScope {
    return session.isCurrent(variables) && variables.roleName === roleName;
  }

  function updateRoleSnapshot(variables: RoleMutationScope, updatedRole: Role): void {
    if (!session.isCurrent(variables)) return;
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
      mutationFn: (variables: RoleMutationScope) =>
        completeMutation(
          () => variables.api.deleteRole(variables.roleName),
          variables.canComplete,
          () => {
            removeDeletedRoleQueries(variables.serverId, variables.connection, variables.roleName);
            void goto(
              resolve('/chat/[serverId]/manage/server/permissions', {
                serverId: serverIdToSegment(variables.serverId)
              })
            );
          }
        ),
      onError: (_error, variables) => {
        if (isCurrentRole(variables)) deleteConfirmRoleName = null;
      }
    }),
    () => queryClient
  );

  function mutationScope(targetRole: Role): RoleMutationScope {
    const snapshot = session.snapshot();
    return {
      ...snapshot,
      roleName: targetRole.name,
      queryKey: adminQueryKeys.role(snapshot.serverId, snapshot.connection, targetRole.name),
      api: snapshot.connection.getAPI(createRoleAPI),
      canComplete: captureMutationCompletion(serverScope)
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

  const saving = $derived(metadataMutation.isPending && isCurrentRole(metadataMutation.variables));
  const savingPingable = $derived(
    pingableMutation.isPending && isCurrentRole(pingableMutation.variables)
  );
  const deleting = $derived(deleteMutation.isPending && isCurrentRole(deleteMutation.variables));
  const error = $derived.by(() => {
    if (roleQuery.error) {
      return errorMessage(roleQuery.error);
    }
    if (metadataMutation.isError && isCurrentRole(metadataMutation.variables)) {
      return errorMessage(metadataMutation.error, m('admin.permissions.update_role_failed'));
    }
    if (pingableMutation.isError && isCurrentRole(pingableMutation.variables)) {
      return errorMessage(pingableMutation.error, m('admin.permissions.update_ping_failed'));
    }
    if (deleteMutation.isError && isCurrentRole(deleteMutation.variables)) {
      return errorMessage(deleteMutation.error, m('admin.permissions.delete_role_failed'));
    }
    return null;
  });
</script>

{#snippet roleSubtitle()}
  {#if role}
    <span class="block truncate">{role.displayName}</span>
  {:else}
    <LoadingFog class="h-4 w-28" label={m('admin.permissions.loading_role')} />
  {/if}
{/snippet}

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: role?.displayName ?? m('admin.permissions.edit_role_title')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.permissions.edit_role_title')}
    subtitle={role?.displayName ?? m('admin.permissions.loading_role')}
    subtitleContent={roleSubtitle}
    backHref={permissionsHref}
    backLabel={m('admin.permissions.back_to_permissions')}
  />

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-40 w-full" label={m('admin.permissions.loading_role')} />
      {:else if !role}
        <Hint tone="danger">{m('admin.permissions.role_not_found')}</Hint>
      {:else if !canManageRoles}
        <Hint tone="danger">{m('admin.permissions.need_manage_edit')}</Hint>
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
      {/if}

      <!-- Permissions matrix: full per-role allow/deny across server, groups, and rooms. -->
      {#if loading || (canManageRoles && role)}
        <Hint>
          {#if roleName === 'owner'}
            {m('admin.permissions.owner_permissions_hint')}
          {:else}
            {m('admin.permissions.role_permissions_hint')}
          {/if}
        </Hint>
        <RolePermissionsMatrix {roleName} />
      {/if}

      {#if role && canManageRoles}
        <!-- Users with this role -->
        {#if canAssignRoles || role.name === 'everyone'}
          <Panel
            title={m('admin.permissions.users_with_role')}
            icon="iconify icon-[uil--users-alt]"
            noPadding
          >
            {#if role?.name === 'everyone'}
              <p class="p-5 text-muted">{m('admin.permissions.everyone_implicit')}</p>
            {:else if canAssignRoles}
              {#if membersError}
                <div class="p-5"><FormError error={membersError} /></div>
              {:else}
                <UserList
                  users={roleUsers}
                  loading={membersQuery.isPending}
                  totalCount={membersQuery.data?.pages.at(-1)?.totalCount ?? 0}
                  hasMore={membersQuery.hasNextPage && !membersError}
                  loadingMore={membersQuery.isFetchingNextPage}
                  onLoadMore={loadMoreMembers}
                  loadMoreRoot={scrollContainer}
                  clickable={canAssignRoles}
                  emptyMessage={m('admin.permissions.no_users_with_role')}
                />
              {/if}
            {/if}
          </Panel>
        {/if}
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
