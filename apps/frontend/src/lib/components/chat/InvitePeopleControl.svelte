<script lang="ts">
  import { pushState } from '$app/navigation';
  import { page } from '$app/state';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import InvitePeopleDialog from './InvitePeopleDialog.svelte';

  const scope = useServerScope();
  const modal = $derived(page.state.modal);

  function open() {
    pushState('', { modal: { type: 'issueInvitationCode', serverId: scope.serverId } });
  }
</script>

<!-- @component Member issuance entry. Permission loss removes the dialog and its secret. -->
{#if scope.store.permissions.canInviteUsers && scope.store.isAuthenticated && scope.store.serverInfo.accountCreationPolicy === 'invite_only'}
  <button type="button" class="sidebar-item" onclick={open}>
    <span class="iconify sidebar-icon icon-[uil--envelope-share]" aria-hidden="true"></span>
    {m('chat.invitation.title')}
  </button>
  {#if modal?.type === 'issueInvitationCode' && modal.serverId === scope.serverId}
    <InvitePeopleDialog onclose={() => history.back()} />
  {/if}
{/if}
