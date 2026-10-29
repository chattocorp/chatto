<script lang="ts">
  import {
    AccountDeletionAvailability,
    accountDeletionAvailabilityWithFallback
  } from '@chatto/client/api/actionAvailability';
  import { queryClient } from '$lib/query/queryClient';
  import { settingsQueryKeys } from '$lib/query/settings';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { resolve } from '$app/paths';
  import { createAccountAPI } from '@chatto/client/api/account';
  import { Panel, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import DeleteAccountSection from './DeleteAccountSection.svelte';
  import ExternalIdentitySettings from './ExternalIdentitySettings.svelte';
  import PasswordSettings from './PasswordSettings.svelte';
  import VerifiedEmailSettings from './VerifiedEmailSettings.svelte';

  const serverScope = useServerScope();
  const currentUser = $derived(serverScope.store.currentUser);
  const serverId = serverScope.serverId;
  const serverSegment = $derived(serverIdToSegment(serverId));
  const accountSettingsPath = $derived(
    resolve('/chat/[serverId]/settings/account', { serverId: serverSegment })
  );

  function accountAPI() {
    return serverScope.connection.getAPI(createAccountAPI);
  }
</script>

<PageTitle title={m('settings.account.title')} />

<div class="pane-page">
  <PaneHeader title={m('settings.account.title')} subtitle={m('settings.account.subtitle')} />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel title={m('settings.account.info_title')} icon="iconify icon-[uil--info-circle]">
        <dl class="flex max-w-md flex-col gap-3 text-sm">
          <div class="flex items-center justify-between">
            <dt class="text-muted">{m('common.user_id')}</dt>
            <dd class="font-mono">{serverScope.store.accountId}</dd>
          </div>
          <div class="flex items-center justify-between">
            <dt class="text-muted">{m('settings.account.username')}</dt>
            <dd class="font-mono">{currentUser.user?.login}</dd>
          </div>
          <div class="flex items-center justify-between">
            <dt class="text-muted">{m('settings.account.display_name')}</dt>
            <dd>
              <AccountName name={currentUser.user?.displayName ?? ''} identity={currentUser.user} />
            </dd>
          </div>
        </dl>
      </Panel>

      <PasswordSettings
        {currentUser}
        directLoginEnabled={serverScope.store.serverInfo.directLoginEnabled}
        getAccountAPI={accountAPI}
        onPasswordSet={() =>
          queryClient.invalidateQueries({
            queryKey: settingsQueryKeys.externalIdentities(
              serverScope.serverId,
              serverScope.connection,
              serverScope.store.accountId ?? ''
            )
          })}
      />
      {#key serverScope.store.accountId ?? ''}
        <VerifiedEmailSettings />
      {/key}
      <ExternalIdentitySettings {currentUser} {accountSettingsPath} />
      <DeleteAccountSection
        canDeleteAccount={accountDeletionAvailabilityWithFallback(
          currentUser.user?.actions?.deleteAccount,
          currentUser.user?.viewerCanDeleteAccount ?? false
        ) === AccountDeletionAvailability.ALLOWED}
        getAccountAPI={accountAPI}
      />
    </div>
  </PaneContent>
</div>
