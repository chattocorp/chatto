<!-- @component Presents an invitation without loading protected room content. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { createMemberDirectoryAPI } from '@chatto/client/api/memberDirectory';
  import { createRoomInvitationAPI } from '@chatto/client/api/roomInvitations';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import type { RoomsListItem } from '$lib/state/server/navigation';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { PageTitle } from '$lib/ui';
  import { Button, FormError } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

  let { room, serverSegment }: { room: RoomsListItem; serverSegment: string } = $props();
  const scope = useServerScope();
  const session = createSessionGuard(scope);
  const invitation = $derived(room.viewerInvitation);
  let answering = $state(false);
  let error = $state<string | null>(null);
  const sender = createQuery(() => {
    const senderId = invitation?.senderId ?? '';
    return {
      queryKey: [
        ...serverSessionQueryRoot(scope.serverId, scope.connection),
        'invitation-sender',
        senderId
      ],
      queryFn: () => scope.connection.getAPI(createMemberDirectoryAPI).getUser(senderId),
      enabled: !!senderId
    };
  });

  async function answer(accept: boolean): Promise<void> {
    if (!invitation || answering) return;
    const id = invitation.id;
    const snapshot = session.snapshot();
    answering = true;
    error = null;
    try {
      const api = scope.connection.getAPI(createRoomInvitationAPI);
      if (accept) await api.acceptInvitation(id);
      else await api.declineInvitation(id);
      if (!session.isCurrent(snapshot)) return;
      // The canonical room projection opens the joined room after acceptance.
      if (!accept) await goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
    } catch (cause) {
      if (session.isCurrent(snapshot) && invitation?.id === id) error = errorMessage(cause);
    } finally {
      if (session.isCurrent(snapshot) && invitation?.id === id) answering = false;
    }
  }
</script>

<PageTitle title={`#${room.name}`} />
<section
  class="flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-y-auto bg-background px-4 py-8 sm:px-6 sm:py-10"
>
  <div
    class="flex w-full max-w-lg flex-col items-center panel-shell p-6 text-center panel-shell-raised sm:p-8"
    data-testid="room-invitation-preview"
  >
    <span class="iconify mb-4 icon-[uil--envelope-alt] text-3xl text-action" aria-hidden="true"
    ></span>
    <h1 class="text-2xl font-semibold text-text-top"><bdi>#{room.name}</bdi></h1>
    <p class="mt-4 text-text">{m('room.invitation.invited')}</p>
    {#if !sender.isPending}
      <p class="mt-2 text-muted" dir="auto">
        {m('room.invitation.sender', {
          name: sender.data?.displayName || sender.data?.login || m('common.unknown_user')
        })}
      </p>
    {/if}
    {#if invitation}
      <p class="mt-2 text-muted">
        {m('room.invitation.expires', {
          date: formatDateTime(
            invitation.expiresAt,
            timeFormatSettingsFor(scope.store.currentUser.user?.settings)
          )
        })}
      </p>
      {#if !invitation.viewerCanAccept}
        <p class="mt-4 text-muted">{m('room.invitation.unavailable')}</p>
      {/if}
      <FormError {error} />
      <div class="mt-6 flex flex-wrap justify-center gap-2">
        <Button
          loading={answering}
          disabled={!invitation.viewerCanAccept}
          onclick={() => void answer(true)}>{m('room.invitation.accept')}</Button
        >
        <Button variant="secondary" disabled={answering} onclick={() => void answer(false)}
          >{m('room.invitation.decline')}</Button
        >
      </div>
    {/if}
  </div>
</section>
