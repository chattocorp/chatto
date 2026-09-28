<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { goto } from '$app/navigation';
  import { captureMutationCompletion, completeMutation } from '$lib/navigation/mutationCompletion';
  import { resolve } from '$app/paths';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createRoleAPI, type CreateRoleInput } from '$lib/api-client/roles';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { RoleForm } from '$lib/components/rbac';
  import { invalidatePermissionTiers } from '$lib/query/adminInvalidation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const session = createSessionGuard(serverScope);

  let name = $state('');
  let displayName = $state('');
  let description = $state('');
  let pingable = $state(false);

  type CreateRoleVariables = SessionSnapshot & {
    api: ReturnType<typeof createRoleAPI>;
    input: CreateRoleInput;
    canComplete: () => boolean;
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

  const createRoleMutation = createMutation(
    () => ({
      mutationFn: (variables: CreateRoleVariables) =>
        completeMutation(
          () => variables.api.createRole(variables.input),
          variables.canComplete,
          () => {
            invalidatePermissionTiers(variables.serverId, variables.connection);
            void goto(
              resolve('/chat/[serverId]/manage/server/permissions/[name]', {
                serverId: serverIdToSegment(variables.serverId),
                name: variables.input.name
              })
            );
          }
        )
    }),
    () => queryClient
  );

  function createRole() {
    const targetName = name.trim();
    const snapshot = session.snapshot();
    createRoleMutation.mutate({
      ...snapshot,
      api: snapshot.connection.getAPI(createRoleAPI),
      canComplete: captureMutationCompletion(serverScope),
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
    createRoleMutation.isPending && session.isCurrent(createRoleMutation.variables)
  );
  const error = $derived(
    roleCatalogQuery.isError
      ? m('admin.permissions.load_instance_failed')
      : createRoleMutation.isError && session.isCurrent(createRoleMutation.variables)
        ? errorMessage(createRoleMutation.error, m('admin.permissions.load_instance_failed'))
        : null
  );
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: m('admin.permissions.create_role_title')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.permissions.create_role_title')}
    subtitle={m('admin.permissions.create_role_subtitle')}
    backHref={resolve('/chat/[serverId]/manage/server/permissions', {
      serverId: serverIdToSegment(serverScope.serverId)
    })}
    backLabel={m('admin.permissions.back_to_permissions')}
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-32 w-full" />
      {:else if !canManageRoles}
        <Hint tone="danger">{m('admin.permissions.need_manage_create')}</Hint>
      {:else}
        {#if error}
          <FormError {error} />
        {/if}

        <Panel title={m('admin.common.role_details')} icon="iconify icon-[uil--plus-circle]">
          <RoleForm
            bind:name
            bind:displayName
            bind:description
            bind:pingable
            saving={creating}
            submitLabel={m('admin.permissions.create_role_action')}
            savingLabel={m('admin.permissions.creating_role')}
            onSubmit={createRole}
          />
          <p class="mt-4 text-sm text-muted">
            {m('admin.permissions.create_after_hint')}
          </p>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
