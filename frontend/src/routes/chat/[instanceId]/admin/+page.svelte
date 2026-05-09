<script lang="ts">
  import { resolve } from '$app/paths';
  import { graphql } from '$lib/gql';
  import { instanceIdToSegment } from '$lib/navigation';
  import { useQuery } from '$lib/hooks';
  import { getAdminPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { StatCard, Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const getInstanceId = getActiveInstance();

  // Get permissions context from layout
  const adminPerms = getAdminPermissions();
  const canViewUsers = $derived(adminPerms.hasPermission('admin.view-users'));

  const usersQuery = useQuery(
    graphql(`
      query AdminDashboardUsers {
        users {
          id
        }
      }
    `),
    () => ({}),
    { skip: () => !canViewUsers }
  );

  const usersCount = $derived(usersQuery.data?.users?.length ?? 0);
  const loading = $derived(usersQuery.loading);
</script>

<PageTitle title="Admin Dashboard" />

<PaneHeader title="Dashboard" subtitle="Instance overview and statistics" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if loading}
    <div class="text-muted">Loading statistics...</div>
  {:else}
    {#if canViewUsers}
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard
          value={usersCount}
          label="Registered Users"
          icon="iconify uil--users-alt"
          color="primary"
        />
      </div>

      <Panel title="Quick Actions">
        <div class="flex flex-wrap gap-3">
          <a href={resolve('/chat/[instanceId]/admin/users', { instanceId: instanceIdToSegment(getInstanceId()) })} class="btn-secondary">
            <span class="iconify uil--users-alt"></span>
            Manage Users
          </a>
        </div>
      </Panel>
    {/if}
  {/if}
</div>
