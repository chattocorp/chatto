<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { AdminMember } from '$lib/api/adminUsers';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { m } from '$lib/i18n/messages';
  import MemberDeleteDialog from './MemberDeleteDialog.svelte';
  import { Panel, ConfirmDialog, UserCard } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toastError } from '$lib/utils/errorMessage';

  let {
    member,
    canDelete,
    changeDeactivation,
    deleteMember
  }: {
    member: AdminMember;
    canDelete: boolean;
    changeDeactivation: (deactivated: boolean) => Promise<boolean>;
    deleteMember: () => Promise<void>;
  } = $props();
  let confirming = $state(false);
  let saving = $state(false);
  let confirmingDeletion = $state(false);
  const action = $derived(
    member.deactivated ? m('admin.members.reactivate') : m('admin.members.deactivate')
  );

  async function confirmDeactivation() {
    if (saving || !member.viewerCanChangeDeactivation) return;
    saving = true;
    try {
      if (await changeDeactivation(!member.deactivated)) confirming = false;
    } catch (error) {
      toastError(error, m('admin.members.deactivation_failed'));
    } finally {
      saving = false;
    }
  }
</script>

<!-- @component Account access controls and the entry point to account deletion. -->
<Panel title={m('admin.members.danger_zone')} icon="iconify icon-[uil--exclamation-triangle]">
  <div class="flex flex-wrap gap-6">
    {#if member.viewerCanChangeDeactivation}
      <div class="flex min-w-0 flex-1 basis-80 flex-col items-start">
        <p class="mb-4 flex-1 text-muted">
          {member.deactivated
            ? m('admin.members.deactivated_description')
            : m('admin.members.deactivate_description')}
        </p>
        <Button variant="secondary" onclick={() => (confirming = true)}>{action}</Button>
      </div>
    {/if}
    {#if canDelete}
      <div class="flex min-w-0 flex-1 basis-80 flex-col items-start">
        <p class="mb-4 flex-1 text-muted">
          {m('admin.members.delete_account_entry_description')}
        </p>
        <Button variant="danger" onclick={() => (confirmingDeletion = true)}
          >{m('admin.members.open_delete_account')}</Button
        >
      </div>
    {/if}
  </div>
</Panel>

{#if confirmingDeletion && canDelete}
  <MemberDeleteDialog {member} {deleteMember} onclose={() => (confirmingDeletion = false)} />
{/if}

{#if confirming}
  <ConfirmDialog
    title={action}
    actionLabel={action}
    tone="warning"
    loading={saving}
    onconfirm={confirmDeactivation}
    onclose={() => {
      if (!saving) confirming = false;
    }}
  >
    <UserCard
      variant="card"
      name={member.displayName || member.login}
      identity={member}
      username={member.login}
      class="mb-3"
    >
      {#snippet avatar()}
        <UserAvatar user={{ ...member, presenceStatus: PresenceStatus.OFFLINE }} size="sm" />
      {/snippet}
    </UserCard>
    {member.deactivated
      ? m('admin.members.deactivated_description')
      : m('admin.members.deactivate_description')}
  </ConfirmDialog>
{/if}
