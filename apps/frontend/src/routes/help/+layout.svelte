<!-- @component Public help navigation, independent of a server or account. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import { helpGroups } from '$lib/help/navigation';
  import { m } from '$lib/i18n/messages';
  let { children }: { children: Snippet } = $props();
  const groups = $derived(helpGroups());
  const items = $derived([
    {
      href: resolve('/help'),
      label: m('help.overview'),
      icon: 'iconify icon-[uil--question-circle]'
    }
  ]);
</script>

<ServerSidebar showCurrentUserBar={false}>
  <SidebarNav title={m('ui.help')} {groups} {items} backHref={resolve('/chat')} />
</ServerSidebar>
<div class="flex min-h-0 min-w-0 flex-1 flex-col" data-navigation-content>
  {@render children()}
</div>
