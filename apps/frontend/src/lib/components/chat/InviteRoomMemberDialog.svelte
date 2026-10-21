<!-- @component Selects a server member and sends one room invitation. -->
<script lang="ts">
  import { createMemberDirectoryAPI } from '@chatto/client/api/memberDirectory';
  import { createRoomInvitationAPI } from '@chatto/client/api/roomInvitations';
  import { isBotAccount } from '@chatto/client/timeline/accountName';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import type { RoomsListItem } from '$lib/state/server/navigation';
  import { createQuery } from '$lib/query/client';
  import { directoryQueryKeys } from '$lib/query/directory';
  import { listEligibleRoomMembers, ELIGIBLE_ROOM_MEMBER_LIMIT } from '$lib/query/roomMembers';
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
  let selectedId = $state('');
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
        listEligibleRoomMembers(
          scope.connection.getAPI(createMemberDirectoryAPI),
          targetRoomId,
          querySearch,
          ELIGIBLE_ROOM_MEMBER_LIMIT,
          signal,
          (user) => !isBotAccount(user) && user.id !== scope.store.viewerId
        ),
      enabled: querySearch.length > 0
    };
  });
  const candidates = $derived(query.data ?? []);

  async function send(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!selectedId || sending) return;
    const snapshot = session.snapshot();
    const targetRoom = room.id;
    sending = true;
    error = null;
    let sent = false;
    try {
      await scope.connection
        .getAPI(createRoomInvitationAPI)
        .createInvitation(targetRoom, selectedId);
      if (!session.isCurrent(snapshot) || targetRoom !== room.id) return;
      toast.success(m('room.invitation.sent'));
      sent = true;
    } catch (cause) {
      if (session.isCurrent(snapshot) && targetRoom === room.id) error = errorMessage(cause);
    } finally {
      if (session.isCurrent(snapshot) && targetRoom === room.id) sending = false;
    }
    // Closing clears the parent target immediately. Do not read room props afterwards.
    if (sent) onclose();
  }
</script>

<FormDialog
  visible
  title={m('room.invitation.invite')}
  submitLabel={m('room.invitation.invite')}
  submitIcon="iconify icon-[uil--user-plus]"
  loading={sending}
  disabled={!selectedId}
  {error}
  onsubmit={send}
  {onclose}
>
  <p class="mb-4"><bdi>#{room.name}</bdi></p>
  <Combobox
    id="room-invitation-recipient"
    label={m('room.invitation.invite')}
    placeholder={m('common.user_search_placeholder')}
    bind:value={selectedId}
    bind:text={search}
    items={candidates}
    getValue={(user) => user.id}
    getLabel={(user) => user.displayName || user.login}
    loading={query.isFetching}
    error={query.error ? errorMessage(query.error) : undefined}
    allowFreeform={false}
    emptyMessage={m('common.no_users')}
    clearLabel={m('common.clear')}
  >
    {#snippet item({ item: user })}
      <UserAvatar {user} size="sm" useLiveProfile={false} />
      <AccountName name={user.displayName || user.login} identity={user} />
      <bdi class="min-w-0 truncate text-muted">@{user.login}</bdi>
    {/snippet}
  </Combobox>
</FormDialog>
