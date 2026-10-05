<script lang="ts">
  import { createUserAPI } from '@chatto/client/api/users';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import AvatarSettings from '../AvatarSettings.svelte';
  import ProfileDetailsSettings from '../ProfileDetailsSettings.svelte';

  const serverScope = useServerScope();
</script>

<PageTitle title={m('settings.profile.title')} />

<div class="pane-page">
  <PaneHeader title={m('settings.profile.title')} subtitle={m('settings.profile.subtitle')} />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <AvatarSettings />
      <ProfileDetailsSettings getUserAPI={() => serverScope.connection.getAPI(createUserAPI)} />
    </div>
  </PaneContent>
</div>
