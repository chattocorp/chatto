<script lang="ts">
  import RegistrationSettings from '$lib/components/RegistrationSettings.svelte';
  import SignupMethods from '$lib/components/SignupMethods.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneHeader, PaneContent } from '$lib/ui';
  import { createRegistrationSettingsAPI } from '$lib/api/registrationSettings';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';

  const scope = useServerScope();
  const settings = createQuery(() => ({
    queryKey: adminQueryKeys.registrationSettings(scope.serverId, scope.connection),
    queryFn: ({ signal }) => scope.connection.getAPI(createRegistrationSettingsAPI).get({ signal })
  }));
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: m('chat.management_nav.registration')
  })}
/>
<div class="pane-page">
  <PaneHeader title={m('chat.management_nav.registration')} />
  <PaneContent>
    <div class="flex flex-col gap-6">
      <SignupMethods />
      <RegistrationSettings
        admission={settings.data}
        loading={settings.isPending}
        error={settings.error}
      />
    </div>
  </PaneContent>
</div>
