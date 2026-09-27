<!--
@component

Renders the standard user record table. The caller owns the surrounding panel.
When `clickable` is set, each row links to the member's Server Admin page.
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { CopyId } from '$lib/ui';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import DataTable from '$lib/ui/DataTable.svelte';
  import { m } from '$lib/i18n/messages';

  type User = {
    id: string;
    login: string;
    displayName: string;
    isBot?: boolean;
    deleted?: boolean;
  };

  let {
    users,
    loading = false,
    clickable = true,
    emptyMessage = m('admin.users.empty'),
    totalCount = users.length,
    hasMore = false,
    loadingMore = false,
    onLoadMore,
    loadMoreRoot
  }: {
    users: User[];
    loading?: boolean;
    clickable?: boolean;
    emptyMessage?: string;
    /** Total matching users, including pages not yet loaded. */
    totalCount?: number;
    hasMore?: boolean;
    loadingMore?: boolean;
    onLoadMore?: () => void | Promise<void>;
    loadMoreRoot?: HTMLElement;
  } = $props();

  const serverScope = useServerScope();

  function memberHref(user: User) {
    return resolve('/chat/[serverId]/manage/server/members/[userId]', {
      serverId: serverIdToSegment(serverScope.serverId),
      userId: user.id
    });
  }
</script>

{#if loading}
  <LoadingFog class="m-5 h-32" label={m('admin.users.loading')} />
{:else}
  <DataTable
    items={users}
    columns={3}
    {emptyMessage}
    {hasMore}
    {loadingMore}
    {onLoadMore}
    {loadMoreRoot}
  >
    {#snippet header()}
      <th class="table-header-cell">{m('admin.users.login')}</th>
      <th class="table-header-cell">{m('admin.users.display_name')}</th>
      <th class="table-header-cell">{m('admin.users.id')}</th>
    {/snippet}
    {#snippet row(user: User)}
      <td class="px-4 py-3 font-medium">
        {#if clickable}
          <a class="data-table-row-link" href={memberHref(user)}>{user.login}</a>
        {:else}
          {user.login}
        {/if}
      </td>
      <td class="px-4 py-3"><AccountName name={user.displayName} identity={user} /></td>
      <td class="px-4 py-3 text-muted"><CopyId value={user.id} /></td>
    {/snippet}
  </DataTable>

  <div class="px-5 py-3 text-sm text-muted">
    {m('admin.members.showing', { shown: users.length, total: totalCount })}
  </div>
{/if}
