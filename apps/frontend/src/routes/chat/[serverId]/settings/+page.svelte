<script lang="ts">
  import { createAccountAPI } from '$lib/api-client/account';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PaneHeader } from '$lib/ui';
  import AvatarSettings from './AvatarSettings.svelte';
  import ProfileDetailsSettings from './ProfileDetailsSettings.svelte';

  const serverScope = useServerScope();

  function accountAPI() {
    return serverScope.connection.getAPI(createAccountAPI);
  }
</script>

<PaneHeader
  title={m('settings.profile.title')}
  subtitle={m('settings.profile.subtitle')}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <AvatarSettings getAccountAPI={accountAPI} />
  <ProfileDetailsSettings getAccountAPI={accountAPI} />
</div>
