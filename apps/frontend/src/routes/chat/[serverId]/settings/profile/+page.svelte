<script lang="ts">
  import { createUserAPI } from '@chatto/client/api/users';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import AvatarSettings from '../AvatarSettings.svelte';
  import ProfileDetailsSettings from '../ProfileDetailsSettings.svelte';
  import SettingsColumns from '$lib/components/settings/SettingsColumns.svelte';

  const serverScope = useServerScope();
</script>

<PageTitle title={m('settings.profile.title')} />

<div class="pane-page">
  <PaneHeader title={m('settings.profile.title')} subtitle={m('settings.profile.subtitle')} />

  <PaneContent>
    <SettingsColumns>
      {#snippet start()}
        <AvatarSettings />
      {/snippet}
      {#snippet end()}
        <ProfileDetailsSettings getUserAPI={() => serverScope.connection.getAPI(createUserAPI)} />
      {/snippet}
    </SettingsColumns>
  </PaneContent>
</div>
