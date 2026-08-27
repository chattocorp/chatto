<script lang="ts">
  import type { Snippet } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { browser } from '$app/environment';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { m } from '$lib/i18n/messages';

  let { children }: { children?: Snippet } = $props();

  const navItems = $derived([
    {
      href: resolve('/chat/preferences'),
      label: m('settings.app_preferences.appearance.title'),
      icon: 'iconify icon-[uil--palette]'
    },
    {
      href: resolve('/chat/preferences/language'),
      label: m('settings.preferences.language.title'),
      icon: 'iconify icon-[uil--language]'
    },
    {
      href: resolve('/chat/preferences/composer'),
      label: m('settings.app_preferences.composer.title'),
      icon: 'iconify icon-[uil--edit]'
    }
  ]);

  const authenticatedServerId = $derived.by(() => {
    const activeServerId = getActiveServer();
    if (activeServerId && serverRegistry.isAuthenticated(activeServerId)) return activeServerId;
    return serverRegistry.firstAuthenticatedServerId();
  });

  const canonicalPreferencePath = $derived.by(() => {
    if (!authenticatedServerId) return null;
    const suffix = page.url.pathname.endsWith('/language')
      ? '/language'
      : page.url.pathname.endsWith('/composer')
        ? '/composer'
        : '/app';
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
    <SidebarNav title={m('settings.app_preferences.title')} items={navItems} />
  </ServerSidebar>

  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    {@render children?.()}
  </div>
{/if}
