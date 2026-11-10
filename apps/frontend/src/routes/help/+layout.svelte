<!-- @component Public help navigation, independent of a server or account. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import { helpGroups } from '$lib/help/navigation';
  import { m } from '$lib/i18n/messages';
  let { children }: { children: Snippet } = $props();
  const selectedServer = $derived(page.url.searchParams.get('server'));
  function withServer(href: string) {
    return selectedServer === null ? href : `${href}?server=${encodeURIComponent(selectedServer)}`;
  }
  const groups = $derived(
    helpGroups().map((group) => ({
      ...group,
      items: group.items.map((item) => ({ ...item, href: withServer(item.href) }))
    }))
  );
  const items = $derived([
    {
      href: withServer(resolve('/help')),
      label: m('help.overview'),
      icon: 'iconify icon-[uil--question-circle]'
    },
    {
      href: withServer(resolve('/help/whats-new')),
      label: m('ui.release_highlights.title'),
      icon: 'iconify icon-[uil--star]'
    }
  ]);
</script>

<ServerSidebar showCurrentUserBar={false}>
  <SidebarNav
    title={m('ui.help')}
    {groups}
    {items}
    backHref={resolve('/chat')}
    isActive={(href) => page.url.pathname === href.split('?')[0]}
  />
</ServerSidebar>
<div class="flex min-h-0 min-w-0 flex-1 flex-col" data-navigation-content>
  {@render children()}
</div>
