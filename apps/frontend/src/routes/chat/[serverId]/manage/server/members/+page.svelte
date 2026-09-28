<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { createAdminUserManagementAPI, type AdminRoleSummary } from '$lib/api-client/adminUsers';
  import { Panel, DataTable, Hint, PaneContent, Pill, PaneHeader, PageTitle } from '$lib/ui';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { TextInput } from '$lib/ui/form';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { formatDate as formatDateUtil, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  const PAGE_SIZE = 20;

  let searchInput = $state('');
  let activeSearch = $state('');
  const searchDebounce = useDebounce();
  let scrollContainer = $state<HTMLDivElement>();

  const membersQuery = createInfiniteQuery(() => {
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const search = activeSearch;
    return {
      queryKey: adminQueryKeys.members(serverId, activeConnection, search),
      queryFn: ({ pageParam, signal }) =>
        activeConnection
          .getAPI(createAdminUserManagementAPI)
          .listMembers({ search: search || null, limit: PAGE_SIZE, offset: pageParam }, { signal }),
      initialPageParam: 0,
      getNextPageParam: (lastPage, _pages, lastPageParam) =>
        lastPage.hasMore && lastPage.consumedCount > 0
          ? lastPageParam + lastPage.consumedCount
          : undefined
    };
  });

  const users = $derived.by(() => {
    const seen = new SvelteSet<string>();
    return (membersQuery.data?.pages ?? []).flatMap((page) =>
      page.users.filter((user) => {
        if (seen.has(user.id)) return false;
        seen.add(user.id);
        return true;
      })
    );
  });
  // An empty ID page needs no batch read, so retain the last hydrated labels.
  const roles = $derived<AdminRoleSummary[]>(
    membersQuery.data?.pages.findLast((page) => page.roles.length > 0)?.roles ?? []
  );
  const totalCount = $derived(membersQuery.data?.pages.at(-1)?.totalCount ?? 0);
  const hasMore = $derived(membersQuery.hasNextPage);
  const loading = $derived(membersQuery.isPending);
  const loadingMore = $derived(membersQuery.isFetchingNextPage);
  // The first page replaces the table body with a loading block.
  const initialLoading = $derived(loading && users.length === 0);
  const error = $derived(membersQuery.error ? errorMessage(membersQuery.error) : null);

  function scheduleSearch(event: Event) {
    const value = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    searchInput = value;
    searchDebounce.run(() => {
      const nextSearch = value.trim();
      if (nextSearch === activeSearch) return;
      activeSearch = nextSearch;
    }, 300);
  }

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    await membersQuery.fetchNextPage();
  }

  function getRoleDisplayName(roleName: string): string {
    const role = roles.find((r) => r.name === roleName);
    return role?.displayName || roleName;
  }

  function formatDate(dateStr: string | null | undefined): string {
    if (!dateStr) return '—';
    return formatDateUtil(dateStr, userSettings, activeLocale);
  }

  // Roles to display in the members list. `everyone` is implicit on every
  // authenticated user, so we drop it here — the column would otherwise be
  // dominated by an "Everyone" pill that carries no information.
  function getDisplayRoles(user: (typeof users)[number]): string[] {
    return user.roles.filter((role) => role !== 'everyone');
  }
</script>

<PageTitle title={m('admin.common.page_title', { title: m('admin.members.title') })} />

<div class="pane-page">
  <PaneHeader title={m('admin.members.title')} subtitle={m('admin.members.subtitle')} />

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      {#if error}
        <Hint tone="danger">{error}</Hint>
      {/if}

      <Panel
        title={m('admin.members.title')}
        count={initialLoading ? undefined : totalCount}
        noPadding
      >
        {#snippet actions()}
          <div class="w-48 sm:w-64">
            <TextInput
              label={m('admin.members.search')}
              labelHidden
              leadingIcon="iconify icon-[uil--search]"
              placeholder={m('admin.members.search_placeholder')}
              bind:value={searchInput}
              oninput={scheduleSearch}
            />
          </div>
        {/snippet}
        <DataTable
          items={users}
          columns={5}
          loading={initialLoading}
          loadingMessage={m('admin.members.loading')}
          emptyMessage={m('admin.members.empty')}
          hasMore={hasMore && !error}
          {loadingMore}
          onLoadMore={loadMore}
          loadMoreRoot={scrollContainer}
          loadingMoreMessage={m('admin.members.loading_more')}
        >
          {#snippet header()}
            <th class="table-header-cell">{m('admin.common.user')}</th>
            <th class="table-header-cell">{m('admin.users.login')}</th>
            <th class="table-header-cell">{m('admin.users.email')}</th>
            <th class="table-header-cell">{m('admin.common.joined')}</th>
            <th class="table-header-cell">{m('admin.common.roles')}</th>
          {/snippet}
          {#snippet row(user)}
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <UserAvatar user={{ ...user, presenceStatus: PresenceStatus.OFFLINE }} size="sm" />
                <a
                  class="data-table-row-link min-w-0"
                  href={resolve('/chat/[serverId]/manage/server/members/[userId]', {
                    serverId: serverIdToSegment(serverScope.serverId),
                    userId: user.id
                  })}
                >
                  <AccountName name={user.displayName || user.login} identity={user} />
                </a>
              </div>
            </td>
            <td class="px-4 py-3 text-muted">@{user.login}</td>
            <td class="px-4 py-3 text-muted">
              {#if user.primaryVerifiedEmail}
                <span class="flex min-w-0 items-center gap-1">
                  <span
                    class="iconify icon-[uil--check-circle] shrink-0 text-success"
                    role="img"
                    aria-label={m('admin.members.email_verified')}
                  ></span>
                  <bdi class="truncate" dir="auto" title={user.primaryVerifiedEmail}
                    >{user.primaryVerifiedEmail}</bdi
                  >
                </span>
              {:else}
                —
              {/if}
            </td>
            <td class="px-4 py-3 text-muted">{formatDate(user.createdAt)}</td>
            <td class="px-4 py-3">
              <div class="flex flex-wrap gap-1">
                {#each getDisplayRoles(user) as roleName (roleName)}
                  <Pill>{getRoleDisplayName(roleName)}</Pill>
                {/each}
              </div>
            </td>
          {/snippet}
        </DataTable>
      </Panel>

      {#if !initialLoading}
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="text-sm text-muted">
            {m('admin.members.showing', { shown: users.length, total: totalCount })}
          </div>
        </div>
      {/if}
    </div>
  </PaneContent>
</div>
