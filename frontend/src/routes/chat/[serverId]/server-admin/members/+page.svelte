<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import type { ServerAdminMembersQuery } from '$lib/gql/graphql';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput } from '$lib/ui/form';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();
  const connection = useConnection();
  const PAGE_SIZE = 20;
  const AUTO_LOAD_THRESHOLD_PX = 160;

  const ServerAdminMembersDocument = graphql(`
    query ServerAdminMembers($search: String, $limit: Int!, $offset: Int!) {
      server {
        roles {
          name
          displayName
        }
        members(search: $search, limit: $limit, offset: $offset) {
          users {
            id
            login
            displayName
            avatarUrl
            roles
            createdAt
          }
          totalCount
          hasMore
        }
      }
    }
  `);

  type User = ServerAdminMembersQuery['server']['members']['users'][number];
  type Role = ServerAdminMembersQuery['server']['roles'][number];

  let searchInput = $state('');
  let activeSearch = '';
  let users = $state<User[]>([]);
  let roles = $state<Role[]>([]);
  let totalCount = $state(0);
  let hasMore = $state(false);
  let loading = $state(true);
  let loadingMore = $state(false);
  let error = $state<string | null>(null);
  let requestId = 0;
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  let scrollContainer: HTMLDivElement | undefined;

  onMount(() => {
    void loadFirstPage('');
    return () => clearSearchTimer();
  });

  function clearSearchTimer() {
    if (searchTimer) {
      clearTimeout(searchTimer);
      searchTimer = null;
    }
  }

  function scheduleSearch(event: Event) {
    const value = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    searchInput = value;
    clearSearchTimer();
    searchTimer = setTimeout(() => {
      const nextSearch = value.trim();
      if (nextSearch === activeSearch) return;
      void loadFirstPage(nextSearch);
    }, 300);
  }

  async function queryMembers(search: string, offset: number) {
    return connection()
      .client.query(ServerAdminMembersDocument, {
        search: search || null,
        limit: PAGE_SIZE,
        offset
      })
      .toPromise();
  }

  async function loadFirstPage(search = activeSearch) {
    const currentRequest = ++requestId;
    activeSearch = search;
    loading = true;
    error = null;
    users = [];
    totalCount = 0;
    hasMore = false;

    try {
      const result = await queryMembers(search, 0);
      if (currentRequest !== requestId) return;

      if (result.error) {
        error = result.error.message;
        return;
      }

      if (!result.data?.server) {
        error = 'Server not found';
        return;
      }

      const members = result.data.server.members;
      roles = result.data.server.roles;
      users = members.users;
      totalCount = members.totalCount;
      hasMore = members.hasMore;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load members';
    } finally {
      if (currentRequest === requestId) {
        loading = false;
        await tick();
        maybeLoadMore();
      }
    }
  }

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;

    const currentRequest = ++requestId;
    const search = activeSearch;
    const offset = users.length;
    loadingMore = true;
    error = null;
    let loadedPage = false;

    try {
      const result = await queryMembers(search, offset);
      if (currentRequest !== requestId) return;

      if (result.error) {
        error = result.error.message;
        return;
      }

      if (!result.data?.server) {
        error = 'Server not found';
        return;
      }

      const members = result.data.server.members;
      const seen = new Set(users.map((user) => user.id));
      roles = result.data.server.roles;
      users = [...users, ...members.users.filter((user) => !seen.has(user.id))];
      totalCount = members.totalCount;
      hasMore = members.hasMore;
      loadedPage = true;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load more members';
    } finally {
      if (currentRequest === requestId) {
        loadingMore = false;
        if (loadedPage) {
          await tick();
          maybeLoadMore();
        }
      }
    }
  }

  function isNearScrollEnd(): boolean {
    if (!scrollContainer) return false;
    const scrollableDistance = scrollContainer.scrollHeight - scrollContainer.clientHeight;
    if (scrollableDistance <= 0) return false;
    return scrollableDistance - scrollContainer.scrollTop <= AUTO_LOAD_THRESHOLD_PX;
  }

  function maybeLoadMore() {
    if (isNearScrollEnd()) {
      void loadMore();
    }
  }

  function getRoleDisplayName(roleName: string): string {
    const role = roles.find((r) => r.name === roleName);
    return role?.displayName || roleName;
  }

  function formatDate(dateStr: string | null | undefined): string {
    if (!dateStr) return '—';
    return formatDateUtil(dateStr, userSettings);
  }

  // Roles to display in the members list. `everyone` is implicit on every
  // authenticated user, so we drop it here — the column would otherwise be
  // dominated by an "Everyone" pill that carries no information.
  function getDisplayRoles(user: (typeof users)[number]): string[] {
    return user.roles.filter((role) => role !== 'everyone');
  }
</script>

<PageTitle title="Members | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Members" subtitle="View and manage server members and their roles" showMobileNav />

  <div
    class="min-h-0 flex-1 overflow-y-auto"
    bind:this={scrollContainer}
    onscroll={maybeLoadMore}
  >
    <div class="flex flex-col gap-6 p-6">
      <!-- Search input -->
      <div class="max-w-md">
        <TextInput
          label="Search members"
          placeholder="Search by login or display name..."
          bind:value={searchInput}
          oninput={scheduleSearch}
        />
      </div>

      {#if loading && users.length === 0}
        <div class="text-muted">Loading members...</div>
      {:else}
        {#if error}
          <Hint tone="danger">{error}</Hint>
        {/if}

        <Panel noPadding>
          <DataTable
            items={users}
            columns={4}
            emptyMessage="No members found"
            onRowClick={(user) =>
              goto(
                resolve('/chat/[serverId]/server-admin/members/[userId]', {
                  serverId: serverIdToSegment(getActiveServer()),
                  userId: user.id
                })
              )}
          >
            {#snippet header()}
              <th class="px-4 py-3 font-medium">User</th>
              <th class="px-4 py-3 font-medium">Login</th>
              <th class="px-4 py-3 font-medium">Joined</th>
              <th class="px-4 py-3 font-medium">Roles</th>
            {/snippet}
            {#snippet row(user)}
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  {#if user.avatarUrl}
                    <img src={user.avatarUrl} alt="" class="h-8 w-8 rounded-full object-cover" />
                  {:else}
                    <div
                      class="flex h-8 w-8 items-center justify-center rounded-full bg-surface-200 text-sm"
                    >
                      {user.displayName[0]?.toUpperCase() || '?'}
                    </div>
                  {/if}
                  <span>{user.displayName}</span>
                </div>
              </td>
              <td class="px-4 py-3 text-muted">@{user.login}</td>
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

        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="text-sm text-muted">
            Showing {users.length} of {totalCount} member(s)
          </div>

          {#if loadingMore}
            <div class="flex items-center gap-2 text-sm text-muted" aria-live="polite">
              <span class="iconify text-base uil--spinner animate-spin" aria-hidden="true"></span>
              Loading more members...
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>
