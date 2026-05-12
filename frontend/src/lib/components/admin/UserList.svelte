<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { Panel, DataTable, CopyId } from '$lib/components/admin';

  type User = {
    id: string;
    login: string;
    displayName: string;
    hasVerifiedEmail?: boolean;
    verifiedEmails?: string[];
  };


  let {
    users,
    loading = false,
    clickable = true,
    emptyMessage = 'No users found',
    onUserClick
  }: {
    users: User[];
    loading?: boolean;
    clickable?: boolean;
    emptyMessage?: string;
    onUserClick?: (user: User) => void;
  } = $props();

  function handleRowClick(user: User) {
    if (!clickable) return;
    if (onUserClick) {
      onUserClick(user);
    } else {
      goto(
        resolve('/chat/[serverId]/(chrome)/server-admin/members/[userId]', {
          serverId: serverIdToSegment(getActiveServer()),
          userId: user.id
        })
      );
    }
  }
</script>

{#if loading}
  <div class="text-muted">Loading users...</div>
{:else}
  <Panel noPadding>
    <DataTable
      items={users}
      columns={4}
      {emptyMessage}
      onRowClick={clickable ? handleRowClick : undefined}
    >
      {#snippet header()}
        <th class="px-4 py-3 font-medium">Login</th>
        <th class="px-4 py-3 font-medium">Display Name</th>
        <th class="px-4 py-3 font-medium">Email</th>
        <th class="px-4 py-3 font-medium">ID</th>
      {/snippet}
      {#snippet row(user: User)}
        <td class="px-4 py-3 font-medium">{user.login}</td>
        <td class="px-4 py-3">{user.displayName}</td>
        <td class="px-4 py-3 text-muted">
          {#if user.verifiedEmails && user.verifiedEmails.length > 0}
            <span class="flex items-center gap-1">
              <span class="iconify text-green-500 uil--check-circle"></span>
              {user.verifiedEmails[0]}
              {#if user.verifiedEmails.length > 1}
                <span class="text-xs">+{user.verifiedEmails.length - 1}</span>
              {/if}
            </span>
          {/if}
        </td>
        <td class="px-4 py-3 text-muted"><CopyId value={user.id} /></td>
      {/snippet}
    </DataTable>
  </Panel>

  <div class="text-sm text-muted">{users.length} user(s) total</div>
{/if}
