<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { accountNameToken } from '@chatto/client/timeline/accountName';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import type { AdminMember } from '$lib/api/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { Hint, FormDialog, UserCard } from '$lib/ui';
  import { TextInput } from '$lib/ui/form';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  let {
    member,
    onclose,
    deleteMember
  }: {
    member: AdminMember;
    onclose: () => void;
    deleteMember: () => Promise<void>;
  } = $props();

  /** Wraps an interpolated user value in Unicode isolates so names and logins
   * cannot scramble surrounding RTL sentences. Built from char codes to keep
   * invisible bidirectional control characters out of this source file. */
  function isolate(value: string): string {
    return `${String.fromCharCode(0x2068)}${value}${String.fromCharCode(0x2069)}`;
  }

  let confirmText = $state('');
  let error = $state('');
  let deleting = $state(false);

  const canConfirm = $derived(!deleting && confirmText.length > 0 && confirmText === member.login);

  async function handleSubmit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!canConfirm) return;
    deleting = true;
    error = '';

    try {
      await deleteMember();
      // On success the parent navigates away; keep the busy state while it does.
    } catch (err) {
      error = errorMessage(err, m('admin.member_delete.failed'));
      // Keep the typed confirmation so a retry needs no retyping.
      deleting = false;
    }
  }
</script>

<!-- @component Account deletion confirmation. The shared member layout owns authorization, deletion, cache cleanup, and navigation. -->
<FormDialog
  visible
  title={m('admin.member_delete.title')}
  submitLabel={m('admin.member_delete.submit')}
  submitTone="danger"
  submitIcon="iconify icon-[uil--trash-alt]"
  submitLoadingText={m('admin.member_delete.deleting')}
  loading={deleting}
  disabled={!canConfirm}
  {error}
  onsubmit={handleSubmit}
  onclose={() => {
    if (!deleting) onclose();
  }}
>
  <UserCard
    variant="card"
    name={member.displayName || member.login}
    identity={member}
    username={member.login}
  >
    {#snippet avatar()}
      <UserAvatar user={{ ...member, presenceStatus: PresenceStatus.OFFLINE }} size="sm" />
    {/snippet}
  </UserCard>
  <Hint tone="danger">
    <strong
      ><AccountNameTokens
        text={m('admin.member_delete.warning', { name: accountNameToken(0) })}
        accounts={[{ name: member.displayName, identity: member }]}
      /></strong
    >
  </Hint>

  <div class="space-y-2 text-muted">
    <p>{m('admin.member_delete.consequences_intro')}</p>
    <ul class="list-outside list-disc space-y-1 ps-6">
      <li>{m('admin.member_delete.consequence_rooms')}</li>
      <li>{m('admin.member_delete.consequence_messages')}</li>
      <li>{m('admin.member_delete.consequence_profile')}</li>
      <li>{m('admin.member_delete.consequence_sessions')}</li>
    </ul>
  </div>

  <TextInput
    id="member-delete-confirm"
    label={m('admin.member_delete.confirm_label', { login: isolate(member.login) })}
    bind:value={confirmText}
    placeholder={member.login}
    disabled={deleting}
    autocomplete="off"
  />
</FormDialog>
