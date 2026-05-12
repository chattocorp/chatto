<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';

  const getInstanceId = getActiveServer();
  const instanceSegment = $derived(serverIdToSegment(getInstanceId()));
  const currentUser = $derived(serverRegistry.getStore(getInstanceId()).currentUser);
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import LoadingPage from '$lib/ui/LoadingPage.svelte';

  let { children } = $props();

  // Nav items for settings
  const navItems = $derived([
    { href: resolve('/chat/[serverId]/settings', { serverId: instanceSegment }), label: 'Profile', icon: 'iconify uil--user' },
    { href: resolve('/chat/[serverId]/settings/preferences', { serverId: instanceSegment }), label: 'Preferences', icon: 'iconify uil--clock' },
    { href: resolve('/chat/[serverId]/settings/account', { serverId: instanceSegment }), label: 'Account', icon: 'iconify uil--setting' },
    { href: resolve('/chat/[serverId]/settings/notifications', { serverId: instanceSegment }), label: 'Notifications', icon: 'iconify uil--bell' }
  ]);
</script>

{#if currentUser.loading}
  <LoadingPage />
{:else if !currentUser.user}
  <LoadingPage message="Not logged in" />
{:else}
  <SecondarySidebar width="md:w-56">
    <SidebarNav title="Settings" items={navItems} backHref={resolve('/chat/[serverId]', { serverId: instanceSegment })} />
  </SecondarySidebar>

  <!-- Main content -->
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    {@render children?.()}
  </div>
{/if}
