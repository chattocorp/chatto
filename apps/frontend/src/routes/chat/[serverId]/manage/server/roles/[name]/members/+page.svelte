<!--
@component

Members section of a role: the accounts that hold it. Only viewers who may
assign roles see it. The everyone role has no list, because every member holds
it implicitly.
-->
<script lang="ts">
  import { createRoleAPI } from '@chatto/client/api/roles';
  import { UserList } from '$lib/components/admin';
  import { m } from '$lib/i18n/messages';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, Panel } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { useRoleDetail } from '../roleDetailContext';

  const serverScope = useServerScope();
  const detail = useRoleDetail();
  const canSeeMembers = $derived(detail.canAssignRoles && detail.roleName !== 'everyone');

  const membersQuery = createInfiniteQuery(() => {
    const connection = serverScope.connection;
    const name = detail.roleName;
    return {
      queryKey: adminQueryKeys.roleMembers(serverScope.serverId, connection, name),
      enabled: canSeeMembers,
      queryFn: ({ pageParam, signal }) =>
        connection
          .getAPI(createRoleAPI)
          .listMembers(name, { limit: 20, offset: pageParam }, { signal }),
      initialPageParam: 0,
      getNextPageParam: (lastPage, _pages, offset) =>
        lastPage.hasMore && lastPage.users.length > 0 ? offset + lastPage.users.length : undefined
    };
  });
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
</script>

{#if canSeeMembers}
  <Panel
    title={m('admin.permissions.users_with_role')}
    icon="iconify icon-[uil--users-alt]"
    noPadding
  >
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
        loadMoreRoot={detail.scrollContainer}
        clickable
        emptyMessage={m('admin.permissions.no_users_with_role')}
      />
    {/if}
  </Panel>
{:else}
  <Hint tone="danger">{m('ui.access_denied.message')}</Hint>
{/if}
