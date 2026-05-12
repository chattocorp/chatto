<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput } from '$lib/ui/form';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();

  const SpaceMembersQuery = graphql(`
    query SpaceMembers($search: String) {
      server {
        roles {
          name
          displayName
        }
        members(search: $search, limit: 20) {
          users {
            id
            login
            displayName
            avatarUrl
            roles
            createdAt
          }
          totalCount
        }
      }
    }
  `);

  // Debounced search
  let searchInput = $state('');
  let debouncedSearch = $state('');

  // Debounce search input
  $effect(() => {
    const value = searchInput;
    const timeout = setTimeout(() => {
      debouncedSearch = value;
    }, 300);
    return () => clearTimeout(timeout);
  });

  const membersQuery = useQuery(SpaceMembersQuery, () => ({
    search: debouncedSearch || null
  }));

  let users = $derived(membersQuery.data?.server?.members.users ?? []);
  let totalCount = $derived(membersQuery.data?.server?.members.totalCount ?? 0);
  let roles = $derived(membersQuery.data?.server?.roles ?? []);
  let loading = $derived(membersQuery.loading);
  let error = $derived(
    membersQuery.error ??
      (!membersQuery.loading && !membersQuery.data?.server ? 'Server not found' : null)
  );


  function getRoleDisplayName(roleName: string): string {
    const role = roles.find((r) => r.name === roleName);
    return role?.displayName || roleName;
  }

  function formatDate(dateStr: string | null | undefined): string {
    if (!dateStr) return '—';
    return formatDateUtil(dateStr, userSettings);
  }

  // Get display roles for a user (includes implicit "everyone" role)
  function getDisplayRoles(user: (typeof users)[number]): string[] {
    // Always include "everyone" since membership is implicit
    const displayRoles = ['everyone'];
    // Add any explicit roles (excluding "everyone" if it somehow appears)
    for (const role of user.roles) {
      if (role !== 'everyone' && !displayRoles.includes(role)) {
        displayRoles.push(role);
      }
    }
    return displayRoles;
  }
</script>

<PageTitle title="Members | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Members" subtitle="View and manage server members and their roles" showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <!-- Search input -->
    <div class="max-w-md">
      <TextInput
        label="Search members"
        placeholder="Search by login or display name..."
        bind:value={searchInput}
      />
    </div>

    {#if loading}
      <div class="text-muted">Loading members...</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      <Panel noPadding>
        <DataTable
          items={users}
          columns={4}
          emptyMessage="No members found"
          onRowClick={(user) =>
            goto(
              resolve('/chat/[serverId]/(chrome)/server-admin/members/[userId]', {
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

      <div class="text-sm text-muted">
        Showing {users.length} of {totalCount} member(s)
      </div>
    {/if}
  </div>
</div>
