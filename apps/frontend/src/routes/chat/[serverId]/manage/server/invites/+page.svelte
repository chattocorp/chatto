<script lang="ts">
  import InvitationCodes from '$lib/components/InvitationCodes.svelte';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneHeader, PaneContent, Hint } from '$lib/ui';

  let scrollContainer = $state<HTMLDivElement>();
</script>

<!-- @component Invitation management; the server limits members to their own codes. -->
<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('chat.management_nav.invites') })}
/>
<div class="pane-page">
  <PaneHeader title={m('chat.management_nav.invites')} />
  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      <Hint>{m('admin.invitations.management_help')}</Hint>
      <InvitationCodes {scrollContainer} />
    </div>
  </PaneContent>
</div>
