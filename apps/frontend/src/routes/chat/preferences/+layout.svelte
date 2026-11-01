<script lang="ts">
  import type { Snippet } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { browser } from '$app/environment';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import SettingsSidebar from '$lib/components/settings/SettingsSidebar.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/client';
  import { firstAuthenticatedServerId } from '$lib/serverCatalogue';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';

  let { children }: { children?: Snippet } = $props();

  const groups = $derived(getSettingsNavGroups());

  const authenticatedServerId = $derived.by(() => {
    const activeServerId = getActiveServer();
    if (activeServerId && serverRegistry.isAuthenticated(activeServerId)) return activeServerId;
    return firstAuthenticatedServerId();
  });

  const canonicalPreferencePath = $derived.by(() => {
    if (page.url.pathname === resolve('/chat/preferences/desktop')) return null;
    if (!authenticatedServerId) return null;
    const suffix =
      page.url.pathname === resolve('/chat/preferences')
        ? '/overview'
        : page.url.pathname.endsWith('/language')
          ? '/language'
          : page.url.pathname.endsWith('/composer')
            ? '/composer'
            : '/appearance';
    const settingsPath = resolve('/chat/[serverId]/settings', {
      serverId: serverIdToSegment(authenticatedServerId)
    });
    return `${settingsPath}${suffix}`;
  });

  $effect(() => {
    if (!browser || !canonicalPreferencePath) return;
    void goto(resolve(canonicalPreferencePath as '/'), { replaceState: true });
  });
</script>

{#if !canonicalPreferencePath}
  <ServerSidebar showCurrentUserBar={false}>
    <SettingsSidebar overviewHref={resolve('/chat/preferences')} {groups} />
  </ServerSidebar>

  <div class="flex min-h-0 min-w-0 flex-1 flex-col" data-navigation-content>
    {@render children?.()}
  </div>
{/if}
