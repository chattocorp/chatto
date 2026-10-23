<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { LeaveRoomModalState } from '$lib/modal';
  import { serverIdToSegment } from '$lib/navigation';
  import { createRoomCommandAPI } from '@chatto/client/api/rooms';
  import { serverConnectionManager, serverRegistry } from '$lib/client';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog } from '$lib/ui';

  let {
    modal,
    onclose
  }: {
    modal: LeaveRoomModalState;
    onclose: () => void;
  } = $props();

  let leaving = $state(false);
  const canRejoin = $derived(
    serverRegistry
      .tryGetStore(modal.serverId)
      ?.roomList.rooms.find((room) => room.id === modal.roomId)?.viewerCanRejoin
  );

  async function leaveRoom() {
    leaving = true;
    try {
      const api = serverConnectionManager.getClient(modal.serverId).getAPI(createRoomCommandAPI);
      await api.leaveRoom(modal.roomId);
    } catch (error) {
      toast.error(m('room.leave.failed'));
      console.error('Error leaving room:', error);
      onclose();
      return;
    } finally {
      leaving = false;
    }

    clearLastRoom(modal.serverId);
    goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(modal.serverId) }));
  }
</script>

<ConfirmDialog
  title={m('room.leave.title')}
  actionLabel={m('room.leave.action')}
  actionIcon="iconify icon-[uil--sign-out-alt] rtl:-scale-x-100"
  loading={leaving}
  onconfirm={leaveRoom}
  {onclose}
>
  <p>{m('room.leave.prompt', { room: modal.roomName })}</p>
  <p class="mt-2">
    {m(
      canRejoin === undefined
        ? 'room.leave.rejoin_unknown'
        : canRejoin
          ? 'room.leave.join_later'
          : 'room.leave.need_invite'
    )}
  </p>
</ConfirmDialog>
