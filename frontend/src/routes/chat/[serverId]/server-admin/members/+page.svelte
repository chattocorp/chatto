<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput } from '$lib/ui/form';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';
  import { ListAdminMembersRequest } from '$lib/pb/chatto/api/v1/chat_pb';
  import type { AdminMemberView, AdminRoleView } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

  const userSettings = getUserSettings();
  const PAGE_SIZE = 20;

  type User = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string;
    roles: string[];
    createdAt?: string | null;
  };
  type Role = {
    name: string;
    displayName: string;
  };

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
  let scrollContainer = $state<HTMLDivElement>();

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
    return withActiveServerWireClient((client) =>
      client.listAdminMembers(
        new ListAdminMembersRequest({
          search,
          limit: PAGE_SIZE,
          offset
        })
      )
    );
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

      roles = result.roles.map(roleFromWire);
      users = result.members.map(memberFromWire);
      totalCount = result.totalCount;
      hasMore = result.hasMore;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load members';
    } finally {
      if (currentRequest === requestId) {
        loading = false;
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

    try {
      const result = await queryMembers(search, offset);
      if (currentRequest !== requestId) return;

      const seen = new Set(users.map((user) => user.id));
      const nextUsers = result.members.map(memberFromWire);
      roles = result.roles.map(roleFromWire);
      users = [...users, ...nextUsers.filter((user) => !seen.has(user.id))];
      totalCount = result.totalCount;
      hasMore = result.hasMore;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load more members';
    } finally {
      if (currentRequest === requestId) {
        loadingMore = false;
      }
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

  function memberFromWire(member: AdminMemberView): User {
    return {
      id: member.user?.id ?? '',
      login: member.user?.login ?? '',
      displayName: member.user?.displayName ?? '',
      avatarUrl: member.avatarUrl || undefined,
      roles: [...member.roles],
      createdAt: member.user?.createdAt?.toDate().toISOString() ?? null
    };
  }

  function roleFromWire(role: AdminRoleView): Role {
    return {
      name: role.name,
      displayName: role.displayName
    };
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
  <PaneHeader
    title="Members"
    subtitle="View and manage server members and their roles"
    showMobileNav
  />

  <div class="min-h-0 flex-1 overflow-y-auto" bind:this={scrollContainer}>
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
            hasMore={hasMore && !error}
            {loadingMore}
            onLoadMore={loadMore}
            loadMoreRoot={scrollContainer}
            loadingMoreMessage="Loading more members..."
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
        </div>
      {/if}
    </div>
  </div>
</div>
