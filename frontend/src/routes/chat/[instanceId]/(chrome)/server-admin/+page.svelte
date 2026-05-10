<script lang="ts">
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import { StatCard, Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const getInstanceId = getActiveInstance();
  const instancePerms = getInstancePermissions();
  const canViewUsers = $derived(instancePerms.current.canAdminViewUsers);

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
  // Only treat the query as loading when it's actually firing — when the
  // viewer can't see users we skip the query entirely and want the
  // "select a section" placeholder to render immediately.
  const loading = $derived(canViewUsers && usersQuery.loading);
</script>

<PageTitle title="Admin Dashboard" />

<PaneHeader title="Dashboard" subtitle="Server overview and statistics" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if loading}
    <div class="text-muted">Loading statistics...</div>
  {:else if canViewUsers}
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
        <a
          href={resolve('/chat/[instanceId]/(chrome)/server-admin/members', { instanceId: instanceIdToSegment(getInstanceId()) })}
          class="btn-secondary"
        >
          <span class="iconify uil--users-alt"></span>
          Manage Users
        </a>
      </div>
    </Panel>
  {:else}
    <div class="flex flex-1 flex-col items-center justify-center gap-4 text-muted">
      <span class="iconify text-6xl uil--setting"></span>
      <p>Select a section from the sidebar to get started.</p>
    </div>
  {/if}
</div>
