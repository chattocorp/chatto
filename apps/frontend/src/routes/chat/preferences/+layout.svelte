<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
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
</script>

<ServerSidebar showCurrentUserBar={false}>
  <SidebarNav title={m('settings.app_preferences.title')} items={navItems} />
</ServerSidebar>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  {@render children?.()}
</div>
