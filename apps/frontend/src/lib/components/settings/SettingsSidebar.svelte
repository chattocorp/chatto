<!-- @component Shared settings navigation. The route supplies the sections available to the viewer. -->
<script lang="ts">
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import { m } from '$lib/i18n/messages';
  import type { SettingsNavGroup } from '$lib/navigation/settingsNav';

  let {
    groups,
    overviewHref,
    subtitle,
    backHref
  }: {
    groups: SettingsNavGroup[];
    overviewHref: string;
    subtitle?: string;
    backHref?: string;
  } = $props();

  const overviewNavItems = $derived([
    {
      href: overviewHref,
      label: m('settings.overview.title'),
      icon: 'iconify icon-[uil--setting]'
    }
  ]);
</script>

<SidebarNav
  title={m('settings.nav.title')}
  {subtitle}
  items={overviewNavItems}
  {groups}
  {backHref}
  backLabel={m('settings.nav.back_to_server')}
/>
