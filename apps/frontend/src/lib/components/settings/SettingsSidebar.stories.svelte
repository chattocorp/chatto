<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { resolve } from '$app/paths';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import SettingsSidebar from './SettingsSidebar.svelte';

  const { Story } = defineMeta({ title: 'Settings/Sidebar', component: SettingsSidebar });
</script>

<Story name="Signed out" asChild>
  <div class="flex h-[36rem] w-72 flex-col bg-background">
    <SettingsSidebar overviewHref={resolve('/chat/preferences')} groups={getSettingsNavGroups()} />
  </div>
</Story>

<Story name="Signed in" asChild>
  <div class="flex h-[36rem] w-72 flex-col bg-background">
    <SettingsSidebar
      overviewHref={resolve('/chat/[serverId]/settings/overview', { serverId: 'preview' })}
      subtitle="Preview server"
      backHref={resolve('/chat/[serverId]', { serverId: 'preview' })}
      groups={getSettingsNavGroups({
        serverId: 'preview',
        serverSegment: 'preview',
        managementNavItems: []
      })}
    />
  </div>
</Story>
