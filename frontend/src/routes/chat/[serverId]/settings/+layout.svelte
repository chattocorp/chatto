<script lang="ts">
  import { resolve } from '$app/paths';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const getServerId = getActiveServer();
  const serverSegment = $derived(serverIdToSegment(getServerId()));
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import LoadingPage from '$lib/ui/LoadingPage.svelte';

  let { children } = $props();

  const currentUser = getCurrentUser();

  // Nav items for settings
  const navItems = $derived([
    { href: resolve('/chat/[serverId]/settings', { serverId: serverSegment }), label: 'Profile', icon: 'iconify uil--user' },
    { href: resolve('/chat/[serverId]/settings/preferences', { serverId: serverSegment }), label: 'Preferences', icon: 'iconify uil--clock' },
    { href: resolve('/chat/[serverId]/settings/account', { serverId: serverSegment }), label: 'Account', icon: 'iconify uil--setting' },
    { href: resolve('/chat/[serverId]/settings/notifications', { serverId: serverSegment }), label: 'Notifications', icon: 'iconify uil--bell' }
  ]);
</script>

{#if currentUser.loading}
  <LoadingPage />
{:else if !currentUser.user}
  <LoadingPage message="Not logged in" />
{:else}
  <SecondarySidebar width="md:w-56">
    <SidebarNav title="Settings" items={navItems} backHref={resolve('/chat/[serverId]', { serverId: serverSegment })} />
  </SecondarySidebar>

  <!-- Main content -->
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    {@render children?.()}
  </div>
{/if}
