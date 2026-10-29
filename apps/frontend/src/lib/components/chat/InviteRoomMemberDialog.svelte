<!-- @component Selects server members and sends independent room invitations. -->
<script lang="ts">
  import { createMemberDirectoryAPI } from '@chatto/client/api/memberDirectory';
  import { createRoomInvitationAPI } from '@chatto/client/api/roomInvitations';
  import { isBotAccount } from '@chatto/client/timeline/accountName';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import type { RoomsListItem } from '$lib/state/server/navigation';
  import { createQuery } from '$lib/query/client';
  import { directoryQueryKeys } from '$lib/query/directory';
  import {
    listRoomMemberCandidates,
    ELIGIBLE_ROOM_MEMBER_LIMIT,
    type RoomMemberCandidate
  } from '$lib/query/roomMembers';
  import { FormDialog } from '$lib/ui';
  import { Combobox } from '$lib/ui/form';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { m } from '$lib/i18n/messages';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { toast } from '$lib/ui/toast';

  let { room, onclose }: { room: RoomsListItem; onclose: () => void } = $props();
  const scope = useServerScope();
  const session = createSessionGuard(scope);
  let selected = $state<RoomMemberCandidate[]>([]);
  let search = $state('');
  let sending = $state(false);
  let error = $state<string | null>(null);
  const query = createQuery(() => {
    const targetRoomId = room.id;
    const querySearch = search.trim();
    return {
      queryKey: [
        ...directoryQueryKeys.eligibleRoomMembers(
          scope.serverId,
          scope.connection,
          targetRoomId,
          querySearch,
          ELIGIBLE_ROOM_MEMBER_LIMIT
        ),
        'human-invitations'
      ],
      queryFn: ({ signal }) =>
        listRoomMemberCandidates(
          scope.connection.getAPI(createMemberDirectoryAPI),
          targetRoomId,
          querySearch,
          ELIGIBLE_ROOM_MEMBER_LIMIT,
          signal,
          (user) => !isBotAccount(user)
        ),
      enabled: querySearch.length > 0
    };
  });
  const candidates = $derived((query.data ?? []).filter(isActive));
  const activeSelected = $derived(selected.filter(isActive));

  function isActive(user: RoomMemberCandidate): boolean {
    return !(scope.store.projection.users.get(user.id)?.user?.deactivated ?? user.deactivated);
  }

  /** Invalidate pending sends before the parent clears the dialog's room target. */
  function close(): void {
    session.invalidate();
    onclose();
  }

  async function send(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!activeSelected.length || sending) return;
    const snapshot = session.snapshot();
    const targetRoom = room.id;
    const recipients = [...activeSelected];
    const failures: string[] = [];
    sending = true;
    error = null;
    try {
      // Keep at most one request in flight. Completed recipients leave the draft,
      // so a partial failure can be retried without sending successful invites again.
      for (const recipient of recipients) {
        if (!session.isCurrent(snapshot) || targetRoom !== room.id) return;
        try {
          await scope.connection
            .getAPI(createRoomInvitationAPI)
            .createInvitation(targetRoom, recipient.id);
          if (!session.isCurrent(snapshot) || targetRoom !== room.id) return;
          selected = selected.filter((user) => user.id !== recipient.id);
        } catch (cause) {
          if (!session.isCurrent(snapshot) || targetRoom !== room.id) return;
          failures.push(
            m('room.invitation.failed', {
              name: recipient.displayName || recipient.login,
              error: errorMessage(cause)
            })
          );
        }
      }
      error = failures.length ? failures.join(' ') : null;
    } finally {
      if (session.isCurrent(snapshot) && targetRoom === room.id) sending = false;
    }
    if (!session.isCurrent(snapshot) || targetRoom !== room.id) return;
    if (!selected.length) {
      toast.success(m('room.invitation.sent'));
      close();
    }
  }
</script>

<FormDialog
  visible
  title={m('room.invitation.title', { room: room.name })}
  submitLabel={m('room.invitation.send')}
  submitIcon="iconify icon-[uil--user-plus]"
  loading={sending}
  disabled={!activeSelected.length}
  {error}
  onsubmit={send}
  oncloserequest={() => session.invalidate()}
  onclose={close}
>
  {#snippet description()}
    {m('room.invitation.description')}
  {/snippet}
  <Combobox
    id="room-invitation-recipient"
    label={m('room.invitation.members')}
    placeholder={m('common.user_search_placeholder')}
    multiple
    bind:selectedItems={selected}
    disabled={sending}
    bind:text={search}
    items={candidates}
    getOptionDisabled={(user) => user.alreadyMember}
    getValue={(user) => user.id}
    getLabel={(user) => user.displayName || user.login}
    loading={query.isFetching}
    error={query.error ? errorMessage(query.error) : undefined}
    allowFreeform={false}
    emptyMessage={m('common.no_users')}
    clearLabel={m('common.clear')}
  >
    {#snippet selectedItemLeading({ item: user })}
      <UserAvatar {user} size="xs" useLiveProfile={false} />
    {/snippet}
    {#snippet item({ item: user })}
      <UserAvatar {user} size="sm" useLiveProfile={false} />
      <span class="flex min-w-0 flex-1 flex-col items-start">
        <AccountName name={user.displayName || user.login} identity={user} />
        {#if user.alreadyMember}
          <span class="text-xs text-muted">{m('room.invitation.already_member')}</span>
        {:else}
          <bdi class="max-w-full truncate text-xs text-muted">@{user.login}</bdi>
        {/if}
      </span>
    {/snippet}
  </Combobox>
</FormDialog>
