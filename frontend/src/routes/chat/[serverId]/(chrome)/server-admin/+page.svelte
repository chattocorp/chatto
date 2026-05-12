<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { StatCard } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const instancePerms = getServerPermissions();
  const canViewAdmin = $derived(instancePerms.current.canAdminViewUsers);

  const statsQuery = useQuery(
    graphql(`
      query AdminDashboardStats {
        admin {
          systemInfo {
            stats {
              userCount
              channelRoomCount
              dmRoomCount
            }
          }
        }
      }
    `),
    () => ({}),
    { skip: () => !canViewAdmin }
  );

  const stats = $derived(statsQuery.data?.admin?.systemInfo?.stats);
  const loading = $derived(canViewAdmin && statsQuery.loading);
</script>

<PageTitle title="Admin Dashboard" />

<PaneHeader title="Dashboard" subtitle="Server overview and statistics" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if loading}
    <div class="text-muted">Loading statistics...</div>
  {:else if canViewAdmin && stats}
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
      <StatCard
        value={stats.userCount}
        label="Registered Users"
        icon="iconify uil--users-alt"
        color="primary"
      />
      <StatCard
        value={stats.channelRoomCount}
        label="Channel Rooms"
        icon="iconify uil--comments"
        color="primary"
      />
      <StatCard
        value={stats.dmRoomCount}
        label="DM Rooms"
        icon="iconify uil--envelope"
        color="primary"
      />
    </div>
  {:else}
    <div class="flex flex-1 flex-col items-center justify-center gap-4 text-muted">
      <span class="iconify text-6xl uil--setting"></span>
      <p>Select a section from the sidebar to get started.</p>
    </div>
  {/if}
</div>
