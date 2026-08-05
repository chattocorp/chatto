<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createRoleAPI, type CreateRoleInput } from '$lib/api-client/roles';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { Panel } from '$lib/components/admin';
  import { PaneContent } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { RoleForm } from '$lib/components/rbac';
  import { invalidatePermissionTiers } from '$lib/query/adminInvalidation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import * as m from '$lib/i18n/messages';

  const serverScope = useServerScope();
  let privacyGeneration = 0;
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    privacyGeneration += 1;
    removeCacheRemovalListener();
  });

  let name = $state('');
  let displayName = $state('');
  let description = $state('');
  let pingable = $state(false);

  type CreateRoleVariables = {
    serverId: string;
    connection: ServerConnection;
    api: ReturnType<typeof createRoleAPI>;
    input: CreateRoleInput;
    privacyGeneration: number;
  };

  const roleCatalogQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.roleCatalog(serverId, connection),
        queryFn: ({ signal }) => connection.getAPI(createRoleAPI).listAdminRoles({ signal })
      };
    },
    () => queryClient
  );

  function isCurrentSession(
    variables: CreateRoleVariables | undefined
  ): variables is CreateRoleVariables {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  const createRoleMutation = createMutation(
    () => ({
      mutationFn: ({ api, input }: CreateRoleVariables) => api.createRole(input),
      onSuccess: (createdRole, variables) => {
        if (!isCurrentSession(variables)) return;
        invalidatePermissionTiers(variables.serverId, variables.connection);
        goto(
          resolve('/chat/[serverId]/manage/server/permissions/[name]', {
            serverId: serverIdToSegment(variables.serverId),
            name: createdRole.name
          })
        );
      }
    }),
    () => queryClient
  );

  function createRole() {
    const targetServerId = serverScope.serverId;
    const targetName = name.trim();
    const connection = serverScope.connection;
    createRoleMutation.mutate({
      serverId: targetServerId,
      connection,
      api: connection.getAPI(createRoleAPI),
      privacyGeneration,
      input: {
        name: targetName,
        displayName: displayName.trim(),
        description: description.trim(),
        pingable
      }
    });
  }

  const canManageRoles = $derived(roleCatalogQuery.data?.viewerCanManageRoles ?? false);
  const loading = $derived(roleCatalogQuery.isPending);
  const creating = $derived(
    createRoleMutation.isPending && isCurrentSession(createRoleMutation.variables)
  );
  const error = $derived(
    roleCatalogQuery.isError
      ? m['admin.permissions.load_instance_failed']()
      : createRoleMutation.isError && isCurrentSession(createRoleMutation.variables)
        ? createRoleMutation.error instanceof Error
          ? createRoleMutation.error.message
          : m['admin.permissions.load_instance_failed']()
        : null
  );
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({
    title: m['admin.permissions.create_role_title']()
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m['admin.permissions.create_role_title']()}
    subtitle={m['admin.permissions.create_role_subtitle']()}
    backHref={resolve('/chat/[serverId]/manage/server/permissions', {
      serverId: serverIdToSegment(serverScope.serverId)
    })}
    backLabel={m['admin.permissions.back_to_permissions']()}
    showMobileNav
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
    {#if loading}
      <div class="text-muted">{m['admin.common.loading']()}</div>
    {:else if !canManageRoles}
      <div class="text-danger">
        {m['admin.permissions.need_manage_create']()}
      </div>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <Panel title={m['admin.common.role_details']()} icon="iconify icon-[uil--plus-circle]">
        <RoleForm
          bind:name
          bind:displayName
          bind:description
          bind:pingable
          saving={creating}
          submitLabel={m['admin.permissions.create_role_action']()}
          savingLabel={m['admin.permissions.creating_role']()}
          onSubmit={createRole}
        />
        <p class="mt-4 text-sm text-muted">
          {m['admin.permissions.create_after_hint']()}
        </p>
      </Panel>
    {/if}
    </div>
  </PaneContent>
</div>
