<script lang="ts">
  import { resolve } from '$app/paths';
  import { createAccountAPI } from '$lib/api-client/account';
  import Panel from '$lib/ui/Panel.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PaneContent, PaneHeader } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import DeleteAccountSection from './DeleteAccountSection.svelte';
  import ExternalIdentitySettings from './ExternalIdentitySettings.svelte';
  import PasswordSettings from './PasswordSettings.svelte';

  const serverScope = useServerScope();
  const currentUser = $derived(serverScope.store.currentUser);
  const serverId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(serverId));
  const accountSettingsPath = $derived(
    resolve('/chat/[serverId]/settings/account', { serverId: serverSegment })
  );

  function accountAPI() {
    return serverScope.connection.getAPI(createAccountAPI);
  }
</script>

<PaneHeader
  title={m('settings.account.title')}
  subtitle={m('settings.account.subtitle')}
  showMobileNav
/>

<PaneContent>
  <div class="flex flex-col gap-6">
    <Panel title={m('settings.account.info_title')} icon="iconify icon-[uil--info-circle]">
      <dl class="flex max-w-md flex-col gap-3 text-sm">
        <div class="flex items-center justify-between">
          <dt class="text-muted">{m('admin.members.user_id')}</dt>
          <dd class="font-mono">{currentUser.user?.id}</dd>
        </div>
        <div class="flex items-center justify-between">
          <dt class="text-muted">{m('settings.account.username')}</dt>
          <dd class="font-mono">{currentUser.user?.login}</dd>
        </div>
        <div class="flex items-center justify-between">
          <dt class="text-muted">{m('settings.account.display_name')}</dt>
          <dd>{currentUser.user?.displayName}</dd>
        </div>
      </dl>
    </Panel>

    <PasswordSettings {currentUser} getAccountAPI={accountAPI} />
    <ExternalIdentitySettings {currentUser} {accountSettingsPath} />
    <DeleteAccountSection
      canDeleteAccount={currentUser.user?.viewerCanDeleteAccount ?? false}
      getAccountAPI={accountAPI}
    />
  </div>
</PaneContent>
