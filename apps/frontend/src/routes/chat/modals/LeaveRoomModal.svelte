<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { LeaveRoomModalState } from '$lib/modal';
  import { serverIdToSegment } from '$lib/navigation';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';

  let {
    modal,
    onclose
  }: {
    modal: LeaveRoomModalState;
    onclose: () => void;
  } = $props();

  let leaving = $state(false);

  async function leaveRoom() {
    leaving = true;
    try {
      const api = serverConnectionManager.getClient(modal.serverId).getAPI(createRoomCommandAPI);
      await api.leaveRoom(modal.roomId);
    } catch (error) {
      toast.error(m['room.leave.failed']());
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
  title={m['room.leave.title']()}
  actionLabel={m['room.leave.action']()}
  actionIcon="iconify icon-[uil--sign-out-alt]"
  loading={leaving}
  onconfirm={leaveRoom}
  {onclose}
>
  {m['room.leave.prompt']({ room: modal.roomName })}
</ConfirmDialog>
