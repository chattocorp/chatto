<!-- @component Advisory checks of room configuration, using existing server-resolved role matrices. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { createQueries } from '@tanstack/svelte-query';
  import { createPermissionAPI } from '@chatto/client/api/permissions';
  import { createRoleAPI } from '@chatto/client/api/roles';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, Hint, LoadingFog, PaneContent, PaneHeader, PageTitle, Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { roomsWithoutRead } from './warnings';

  const scope = useServerScope();
  const { serverId, connection } = scope;
  const serverSegment = serverIdToSegment(serverId);
  const roles = createQuery(() => ({
    queryKey: adminQueryKeys.roleCatalog(serverId, connection),
    queryFn: ({ signal }) => connection.getAPI(createRoleAPI).listAdminRoles({ signal })
  }));
  // Reuse the permission editors' cache keys and the shared privacy boundaries.
  // Each role needs one existing matrix read; no member or message data is loaded.
  const matrices = createQueries(
    () => ({
      queries: (roles.data?.roles ?? []).map((role) => ({
        queryKey: adminQueryKeys.rolePermissions(serverId, connection, role.name),
        queryFn: ({ signal }: { signal: AbortSignal }) =>
          connection.getAPI(createPermissionAPI).getRolePermissionMatrix(role.name, { signal })
      }))
    }),
    () => queryClient
  );
  const loading = $derived(roles.isPending || matrices.some((query) => query.isPending));
  const inspections = $derived(
    matrices.map((query) => ({
      query,
      rooms: query.data ? roomsWithoutRead(query.data) : null
    }))
  );
  // Missing data is an incomplete check, never an all-clear result. Keep this
  // validation outside query selectors so cached editor responses are checked too.
  const incomplete = $derived(
    inspections.some(({ query, rooms }) => query.isSuccess && rooms === null)
  );
  const failure = $derived(roles.error ?? matrices.find((query) => query.isError)?.error);
  const warnings = $derived(
    inspections.flatMap(({ query, rooms }) => {
      const data = query.data;
      if (!data || !rooms) return [];
      const roleLabel =
        roles.data?.roles.find((role) => role.name === data.roleName)?.displayName || data.roleName;
      return rooms.map((room) => ({ room, roleName: data.roleName, roleLabel }));
    })
  );

  function retry() {
    void roles.refetch();
    for (const query of matrices) void query.refetch();
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: m('chat.management_nav.configuration_warnings')
  })}
/>
<div class="pane-page">
  <PaneHeader title={m('chat.management_nav.configuration_warnings')} />
  <PaneContent>
    <div class="flex flex-col gap-6">
      <Hint>{m('admin.configuration_warnings.intro')}</Hint>
      <Panel title={m('admin.configuration_warnings.rule')}>
        <div class="flex flex-col gap-4">
          <p class="text-sm text-muted">{m('admin.configuration_warnings.consequence')}</p>
          {#if failure || incomplete}
            <Hint tone="danger"
              >{failure
                ? errorMessage(failure, m('admin.configuration_warnings.load_failed'))
                : m('admin.configuration_warnings.load_failed')}</Hint
            >
            <Button variant="secondary" onclick={retry}>{m('common.retry')}</Button>
          {:else if loading}
            <LoadingFog class="h-32" />
          {:else if warnings.length === 0}
            <EmptyState title={m('admin.configuration_warnings.empty')} />
          {:else}
            {#each warnings as warning (`${warning.roleName}:${warning.room.id}`)}
              <Hint tone="warning">
                <p class="font-medium">
                  <bdi>{warning.room.label}</bdi> · <bdi>{warning.roleLabel}</bdi>
                </p>
                <a
                  class="link"
                  href={resolve('/chat/[serverId]/manage/server/permissions/[name]', {
                    serverId: serverSegment,
                    name: warning.roleName
                  })}>{m('admin.configuration_warnings.review')}</a
                >
              </Hint>
            {/each}
          {/if}
        </div>
      </Panel>
    </div>
  </PaneContent>
</div>
