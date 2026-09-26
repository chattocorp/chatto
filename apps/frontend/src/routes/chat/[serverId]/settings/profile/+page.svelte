<script lang="ts">
  import { createUserAPI } from '$lib/api-client/users';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PaneContent, PaneHeader } from '$lib/ui';
  import AvatarSettings from '../AvatarSettings.svelte';
  import ProfileDetailsSettings from '../ProfileDetailsSettings.svelte';

  const serverScope = useServerScope();
</script>

<PaneHeader
  title={m('settings.profile.title')}
  subtitle={m('settings.profile.subtitle')}
  showMobileNav
/>

<PaneContent>
  <div class="flex flex-col gap-6">
    {#if serverScope.store.serverInfo.supportsFeature('userAvatars')}
      <AvatarSettings />
    {/if}
    <ProfileDetailsSettings getUserAPI={() => serverScope.connection.getAPI(createUserAPI)} />
  </div>
</PaneContent>
