<!-- @component Read-only role navigation for viewers who can inspect role rosters. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { createRoleAPI } from '@chatto/client/api/roles';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { DataTable, Hint, LoadingFog, Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';

  const scope = useServerScope();
  const rolesQuery = createQuery(() => {
    const connection = scope.connection;
    return {
      queryKey: adminQueryKeys.roleCatalog(scope.serverId, connection),
      queryFn: ({ signal }) => connection.getAPI(createRoleAPI).listAdminRoles({ signal })
    };
  });
</script>

<Panel title={m('admin.common.roles')} noPadding>
  {#if rolesQuery.isPending}
    <LoadingFog class="m-5 h-32" label={m('admin.permissions.loading_role')} />
  {:else if rolesQuery.isError}
    <div class="p-5">
      <Hint tone="danger">{errorMessage(rolesQuery.error)}</Hint>
      <Button variant="secondary" onclick={() => rolesQuery.refetch()}>{m('common.retry')}</Button>
    </div>
  {:else}
    <DataTable
      items={rolesQuery.data?.roles ?? []}
      columns={1}
      emptyMessage={m('rbac.permissions.no_roles')}
    >
      {#snippet header()}
        <th class="table-header-cell">{m('rbac.role_form.name')}</th>
      {/snippet}
      {#snippet row(role)}
        <td class="px-4 py-3">
          <a
            class="data-table-row-link"
            href={resolve('/chat/[serverId]/manage/server/permissions/[name]', {
              serverId: serverIdToSegment(scope.serverId),
              name: role.name
            })}
          >
            {role.displayName} (@{role.name})
          </a>
        </td>
      {/snippet}
    </DataTable>
  {/if}
</Panel>
