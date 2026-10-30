<script lang="ts">
  import RegistrationSettings from '$lib/components/RegistrationSettings.svelte';
  import SignupMethods from '$lib/components/SignupMethods.svelte';
  import InvitationCodes from '$lib/components/InvitationCodes.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneHeader, PaneContent } from '$lib/ui';
  import { createRegistrationSettingsAPI } from '$lib/api/registrationSettings';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';

  const scope = useServerScope();
  const permissions = $derived(scope.store.permissions);
  // Both panels use the saved admin value. Code-only managers use public discovery.
  const settings = createQuery(() => ({
    queryKey: adminQueryKeys.registrationSettings(scope.serverId, scope.connection),
    queryFn: ({ signal }) => scope.connection.getAPI(createRegistrationSettingsAPI).get({ signal }),
    enabled: permissions.canManageServer
  }));
  const codesInactive = $derived(
    (permissions.canManageServer ? settings.data : scope.store.serverInfo.accountCreationPolicy) !==
      'invite_only'
  );
  let scrollContainer = $state<HTMLDivElement>();
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: m('chat.management_nav.registration')
  })}
/>
<div class="pane-page">
  <PaneHeader title={m('chat.management_nav.registration')} />
  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      {#if permissions.canManageServer}
        <SignupMethods />
        <RegistrationSettings
          admission={settings.data}
          loading={settings.isPending}
          error={settings.error}
        />
      {/if}
      {#if permissions.canManageInvites}
        <fieldset
          disabled={codesInactive}
          aria-label={m('admin.invitations.list_title')}
          class={['min-w-0', codesInactive && 'opacity-50']}
        >
          <InvitationCodes {scrollContainer} />
        </fieldset>
      {/if}
    </div>
  </PaneContent>
</div>
