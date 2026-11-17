<!-- @component Searchable role catalog shared by managers and viewers of role rosters. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { createRoleAPI } from '@chatto/client/api/roles';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { DataTable, Hint, LoadingFog, Panel, Pill, ToggleChip } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';

  const scope = useServerScope();
  const serverId = serverIdToSegment(scope.serverId);
  let search = $state('');
  const numberFormat = $derived(new Intl.NumberFormat(getLocale()));
  function formatCount(value: number | undefined): string {
    return value === undefined ? '—' : numberFormat.format(value);
  }
  const rolesQuery = createQuery(() => {
    const connection = scope.connection;
    return {
      queryKey: adminQueryKeys.roleCatalog(scope),
      queryFn: ({ signal }) => connection.getAPI(createRoleAPI).listAdminRoles({ signal })
    };
  });
  const roles = $derived.by(() => {
    const term = search.trim().toLowerCase();
    return [...(rolesQuery.data?.roles ?? [])]
      .filter((role) =>
        `${role.displayName} @${role.name} ${role.description}`.toLowerCase().includes(term)
      )
      .sort((a, b) => b.position - a.position);
  });
</script>

<div class="flex flex-col gap-4">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="w-full sm:max-w-xs">
      <TextInput
        label={m('admin.permissions.search_roles')}
        placeholder={m('admin.permissions.search_roles')}
        labelHidden
        leadingIcon="iconify icon-[uil--search]"
        bind:value={search}
      />
    </div>
    {#if rolesQuery.data?.viewerCanManageRoles}
      <Button href={resolve('/chat/[serverId]/manage/server/roles/new', { serverId })}>
        {m('admin.permissions.create_role_action')}
      </Button>
    {/if}
  </div>

  <Panel
    title={m('admin.common.roles')}
    count={rolesQuery.isPending ? undefined : roles.length}
    noPadding
  >
    {#if rolesQuery.isPending}
      <LoadingFog class="m-5 h-32" label={m('admin.permissions.loading_role')} />
    {:else if rolesQuery.isError}
      <div class="flex flex-col items-start gap-3 p-5">
        <Hint tone="danger">{errorMessage(rolesQuery.error)}</Hint>
        <Button variant="secondary" onclick={() => rolesQuery.refetch()}>{m('common.retry')}</Button
        >
      </div>
    {:else}
      <DataTable items={roles} columns={6} getKey={(role) => role.name}>
        {#snippet header()}
          <th class="table-header-cell">{m('admin.permissions.role_tab')}</th>
          <th class="table-header-cell">{m('rbac.role_form.description')}</th>
          <th class="table-header-cell text-end">{m('admin.rooms_admin.tabs.members')}</th>
          <th class="table-header-cell text-end">{m('rbac.permissions.granted')}</th>
          <th class="table-header-cell text-end">{m('rbac.permissions.denied')}</th>
          <th class="table-header-cell"
            ><span class="sr-only">{m('admin.neighbors.actions')}</span></th
          >
        {/snippet}
        {#snippet row(role)}
          <td class="px-4 py-2 whitespace-nowrap">
            <div class="flex items-center gap-3">
              <a
                class="data-table-row-link font-medium"
                href={resolve('/chat/[serverId]/manage/server/roles/[name]', {
                  serverId,
                  name: role.name
                })}><bdi>{role.displayName}</bdi></a
              >
              <bdi class="text-muted">@{role.name}</bdi>
              {#if role.isSystem}<Pill>{m('chat.management_nav.system')}</Pill>{/if}
            </div>
          </td>
          <td class="max-w-64 truncate px-4 py-2 text-muted" title={role.description}>
            <bdi>{role.description}</bdi>
          </td>
          <td class="px-4 py-2 text-end tabular-nums" data-role-member-count
            >{formatCount(role.memberCount)}</td
          >
          <td class="px-4 py-2 text-end text-success tabular-nums" data-role-grant-count
            >{#if role.permissionGrantCount !== 0}{formatCount(role.permissionGrantCount)}{/if}</td
          >
          <td class="px-4 py-2 text-end text-danger tabular-nums" data-role-denial-count
            >{#if role.permissionDenialCount !== 0}{formatCount(
                role.permissionDenialCount
              )}{/if}</td
          >
          <td class="px-3 py-2">
            {#if rolesQuery.data?.viewerCanManageRoles}
              <div class="flex items-center justify-end gap-1">
                <ToggleChip
                  tone="neutral"
                  square
                  title={m('admin.permissions.edit_role_title')}
                  onclick={() =>
                    goto(
                      resolve('/chat/[serverId]/manage/server/roles/[name]', {
                        serverId,
                        name: role.name
                      })
                    )}
                >
                  <span
                    class="iconify icon-[uil--pen] text-base"
                    role="img"
                    aria-label={m('admin.permissions.edit_role_title')}
                  ></span>
                </ToggleChip>
                <ToggleChip
                  tone="neutral"
                  square
                  title={m('admin.permissions.title')}
                  onclick={() =>
                    goto(
                      resolve('/chat/[serverId]/manage/server/roles/[name]/permissions', {
                        serverId,
                        name: role.name
                      })
                    )}
                >
                  <span
                    class="iconify icon-[uil--shield-check] text-base"
                    role="img"
                    aria-label={m('admin.permissions.title')}
                  ></span>
                </ToggleChip>
              </div>
            {/if}
          </td>
        {/snippet}
      </DataTable>
    {/if}
  </Panel>
</div>
