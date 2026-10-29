<script lang="ts">
  import RegistrationSettings from '$lib/components/RegistrationSettings.svelte';
  import InvitationCodes from '$lib/components/InvitationCodes.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneHeader, PaneContent } from '$lib/ui';

  const scope = useServerScope();
  const permissions = $derived(scope.store.permissions);
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
        <RegistrationSettings />
      {/if}
      {#if permissions.canManageInvites}
        <InvitationCodes {scrollContainer} />
      {/if}
    </div>
  </PaneContent>
</div>
